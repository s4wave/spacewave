package prom_plugin

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
)

// commitBatch is an open window encoded for one transaction: the keys and
// values to set in the target's tree.
type commitBatch struct {
	// entries are the key-value pairs to set: the series page, if the window
	// introduced series, and the segment.
	entries []batchEntry
	// endSeries is the number of series in the table once the batch is written.
	endSeries uint64
	// maxTime is the latest sample time in the segment.
	maxTime int64
	// maintain is set when the batch closes a CompactSpan bucket, so its
	// commit also drops expired segments and merges the closed buckets.
	maintain bool
	// samples is the number of samples in the segment.
	samples int
	// expect is the encoded status the target must hold for the batch to be
	// written, or nil when it must hold none.
	expect []byte
	// status is the encoded status the batch writes, or nil.
	status []byte
}

// batchEntry is one key-value pair of a commitBatch.
type batchEntry struct {
	// key is the tree key.
	key []byte
	// value is the marshaled message stored under key.
	value []byte
}

// newCommitBatch encodes w and the series it introduced, which are the table
// entries from w.FirstSeries on. It returns nil for an empty window.
func newCommitBatch(w *Window, table *SeriesTable) (*commitBatch, error) {
	// Encode the window's samples as a segment.
	seg := w.Segment()
	if seg == nil {
		return nil, nil
	}
	segData, err := seg.MarshalVT()
	if err != nil {
		return nil, err
	}
	batch := &commitBatch{
		endSeries: table.Len(),
		maxTime:   seg.GetMaxTime(),
		samples:   w.Samples(),
	}

	// Encode the series the window introduced as a page, written first.
	if added := table.LabelsFrom(w.FirstSeries()); len(added) != 0 {
		page := &SeriesPage{FirstSeries: w.FirstSeries(), Series: make([]*s4wave_prom.LabelSet, 0, len(added))}
		for _, lset := range added {
			page.Series = append(page.Series, NewLabelSet(lset))
		}
		pageData, err := page.MarshalVT()
		if err != nil {
			return nil, err
		}
		batch.entries = append(batch.entries, batchEntry{key: pageKey(w.FirstSeries()), value: pageData})
	}
	batch.entries = append(batch.entries, batchEntry{key: segmentKey(seg.GetMinTime(), seg.GetMaxTime()), value: segData})
	return batch, nil
}

// CommitStores writes each store's open window as one segment, and moves
// every target's root, in one World transaction. It writes nothing for a store
// without samples, and returns zero stats when no store has any.
//
// A write holds only the window's samples and the key-value tree nodes on the
// path to the new keys, so its size follows the window, not the target's
// history. If the transaction fails, no store changes: the next commit retries
// the same windows first. A store whose commit is still running makes
// CommitStores return ErrCommitInProgress without writing. A store whose
// target another writer has taken over writes nothing and stands down, as
// Store.Lost reports. The others commit.
func CommitStores(ctx context.Context, eng world.Engine, stores ...*Store) (CommitStats, error) {
	// Freeze each store's window. Release the ones already frozen on
	// failure.
	var stats CommitStats
	type pending struct {
		store *Store
		batch *commitBatch
	}
	jobs := make([]pending, 0, len(stores))
	release := func() {
		for _, j := range jobs {
			j.store.finish(j.batch, false)
		}
	}
	for _, s := range stores {
		batch, err := s.freeze()
		if err != nil {
			release()
			return stats, errors.Wrapf(err, "target %q", s.Key())
		}
		if batch != nil {
			jobs = append(jobs, pending{store: s, batch: batch})
		}
	}
	if len(jobs) == 0 {
		return stats, nil
	}

	// Write every batch in one transaction, counting the blocks it submits.
	wctx, counter := block.WithWriteCounter(ctx)
	lost := make([]bool, len(jobs))
	err := world.ExecTransaction(wctx, eng, true, func(ctx context.Context, wtx world.WorldState) error {
		for i, j := range jobs {
			err := j.store.write(ctx, wtx, j.batch)
			lost[i] = errors.Is(err, ErrStoodDown)
			if err != nil && !lost[i] {
				return err
			}
		}
		return nil
	})
	if err != nil {
		release()
		return stats, errors.Wrap(err, "commit segments")
	}

	// Publish the written batches as committed state.
	for i, j := range jobs {
		if lost[i] {
			j.store.standDown()
			continue
		}
		j.store.finish(j.batch, true)
		stats.Segments++
		stats.Samples += j.batch.samples
	}
	snap := counter.Snapshot()
	stats.Blocks, stats.Bytes = snap.BlockWriteCount, snap.BlockWriteBytes
	return stats, nil
}
