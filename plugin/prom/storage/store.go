package promstorage

import (
	"bytes"
	"context"
	"math"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/model/value"
	"github.com/prometheus/prometheus/storage"
	"github.com/s4wave/spacewave/db/block"
	block_kvtx "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_prom "github.com/s4wave/spacewave/sdk/prom"
)

// Store is the single writer of one target's samples.
//
// The target is a World object whose root is a block-backed key-value tree.
// The Store keeps the open window and the target's series table in memory.
// Append adds to the window and CommitStores writes the window as one segment.
// A crash or Rollback loses only the uncommitted window. Exactly one Store
// writes a target, so a Store reads the tree after it opens only to maintain
// its segments and to check that it is still the writer: the commit that
// closes a CompactSpan bucket drops expired segments and merges the closed
// buckets' segments in the same transaction, and a commit that finds a status
// this Store did not write stands the Store down instead of writing.
type Store struct {
	// key is the World object key of the target.
	key string
	// meta holds the labels that every series of the target carries.
	meta labels.Labels
	// retention is how long the target keeps samples.
	retention time.Duration

	// mtx guards the fields below.
	mtx sync.Mutex
	// table maps series label sets to series numbers, including series that
	// only the uncommitted windows introduced.
	table *SeriesTable
	// open is the window Append adds to.
	open *Window
	// frozen is the encoded window a commit is writing, or retrying after a
	// failed commit, and is nil when none is pending.
	frozen *commitBatch
	// inFlight is set while a commit of the frozen batch is running.
	inFlight bool
	// committedSeries is the number of series the committed pages hold.
	committedSeries uint64
	// committedMax is the latest sample time committed, valid when hasCommitted.
	committedMax int64
	// hasCommitted is set once the target holds a segment.
	hasCommitted bool
	// status is the encoded TargetStatus the next window commits, or nil.
	status []byte
	// seen is the encoded TargetStatus the target held when this Store last
	// opened or wrote it, or nil if it held none.
	seen []byte
	// lost is closed when a commit stands the Store down.
	lost chan struct{}
	// isLost is set once lost is closed.
	isLost bool
}

// CreateStore creates the World object key as an empty target carrying the
// labels target on every series, and returns its writer.
func CreateStore(ctx context.Context, eng world.Engine, key string, target labels.Labels) (*Store, error) {
	meta := &s4wave_prom.TargetMeta{Labels: NewLabelSet(target)}
	err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, wtx world.WorldState) error {
		return s4wave_prom.CreateTarget(ctx, wtx, key, meta)
	})
	if err != nil {
		return nil, err
	}
	return newStore(key, meta, NewSeriesTable(), 0, false, 0, nil), nil
}

// OpenStore opens the existing target key for writing, reading its labels,
// series table and latest committed time.
func OpenStore(ctx context.Context, eng world.Engine, key string) (*Store, error) {
	// Read the committed state of the target.
	var st *Store
	err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, wtx world.WorldState) error {
		// Access the target's tree.
		obj, err := world.MustGetObject(ctx, wtx, key)
		if err != nil {
			return err
		}
		defer world.ReleaseObjectState(obj)
		_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
			// Open the tree for reading.
			kv, err := block_kvtx.BuildKvTransaction(ctx, bcs, false)
			if err != nil {
				return err
			}
			defer kv.Discard()

			// Read the meta, the series table and the latest committed time.
			r := NewStoreReader(kv)
			meta, err := r.Meta(ctx)
			if err != nil {
				return err
			}
			table, err := r.Table(ctx)
			if err != nil {
				return err
			}
			maxTime, has, err := r.LastSegmentMaxTime(ctx)
			if err != nil {
				return err
			}

			// Read the status this writer must find at its first commit.
			seen, _, err := kv.Get(ctx, s4wave_prom.StatusKey())
			if err != nil {
				return err
			}

			// Resume the writer after the committed state.
			st = newStore(key, meta, table, table.Len(), has, maxTime, seen)
			return nil
		})
		return err
	})
	if err != nil {
		return nil, errors.Wrapf(err, "open target %q", key)
	}
	return st, nil
}

// newStore constructs a store over already committed state, whose target
// held the encoded status seen.
func newStore(key string, meta *s4wave_prom.TargetMeta, table *SeriesTable, committedSeries uint64, hasCommitted bool, committedMax int64, seen []byte) *Store {
	return &Store{
		key:             key,
		meta:            ToLabels(meta.GetLabels()),
		retention:       meta.Retention(),
		table:           table,
		open:            NewWindow(table.Len()),
		committedSeries: committedSeries,
		committedMax:    committedMax,
		hasCommitted:    hasCommitted,
		seen:            seen,
		lost:            make(chan struct{}),
	}
}

// Key returns the World object key of the target.
func (s *Store) Key() string {
	return s.key
}

// WaitScrape waits for the next scrape tick, cancellation, or loss of the writer.
// A commit that finds another writer changed the target's status since this
// Store opened it stands the Store down. WaitScrape then returns ErrStoodDown;
// the writer must reopen the target before writing again.
func (s *Store) WaitScrape(ctx context.Context, tick <-chan time.Time) error {
	select {
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-s.lost:
		return ErrStoodDown
	case <-tick:
		return nil
	}
}

// Append adds a sample of the series lset to the open window.
//
// lset must not carry a name that the target's labels set. The timestamp must
// be later than every sample the target has committed or is committing, and
// later than the series' earlier samples in the open window. Otherwise Append
// returns storage.ErrOutOfOrderSample, or
// storage.ErrDuplicateSampleForTimestamp for the same time with another value.
func (s *Store) Append(lset labels.Labels, t int64, v float64) error {
	// Reject labels that the target already provides.
	var reserved error
	lset.Range(func(l labels.Label) {
		if s.meta.Has(l.Name) {
			reserved = errors.Wrapf(ErrReservedLabel, "%q", l.Name)
		}
	})
	if reserved != nil {
		return reserved
	}

	// Keep each window after the one before it, so segments never overlap.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if floor, ok := s.floorLocked(); ok && t <= floor {
		return storage.ErrOutOfOrderSample
	}

	// Number the series if the window is the first to see it, then append.
	num, ok := s.table.Lookup(lset)
	if !ok {
		num = s.table.Add(lset.Copy())
	}
	return s.open.Append(num, t, v)
}

// AppendStale adds a stale marker for the series lset at time t, which tells
// PromQL the series ended, as Prometheus does for a failed or vanished scrape.
func (s *Store) AppendStale(lset labels.Labels, t int64) error {
	return s.Append(lset, t, math.Float64frombits(value.StaleNaN))
}

// SetStatus records status to commit with the next window that holds samples.
// A later call replaces an uncommitted status.
func (s *Store) SetStatus(status *s4wave_prom.TargetStatus) error {
	// Encode the status and replace the pending one.
	data, err := status.MarshalVT()
	if err != nil {
		return err
	}
	s.mtx.Lock()
	s.status = data
	s.mtx.Unlock()
	return nil
}

// Rollback discards every uncommitted sample and the series only they
// introduced. It returns ErrCommitInProgress while a commit is running.
func (s *Store) Rollback() error {
	// Refuse while a commit is running.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.inFlight {
		return ErrCommitInProgress
	}

	// Forget the frozen batch, the open window and the series they introduced.
	s.frozen = nil
	s.table.Truncate(s.committedSeries)
	s.open = NewWindow(s.committedSeries)
	return nil
}

// floorLocked returns the latest sample time that is committed or frozen.
// Callers hold mtx.
func (s *Store) floorLocked() (int64, bool) {
	floor, ok := s.committedMax, s.hasCommitted
	if s.frozen != nil && (!ok || s.frozen.maxTime > floor) {
		floor, ok = s.frozen.maxTime, true
	}
	return floor, ok
}

// freeze returns the batch a commit should write: the batch left by a failed
// commit, or else the open window encoded as a batch. It returns nil when
// there is nothing to write. A returned batch is in flight until finish.
func (s *Store) freeze() (*commitBatch, error) {
	// Refuse while a commit is running.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.inFlight {
		return nil, ErrCommitInProgress
	}

	// Encode the open window with the pending status, unless a failed commit
	// left a batch to retry.
	if s.frozen == nil {
		batch, err := newCommitBatch(s.open, s.table)
		if err != nil || batch == nil {
			return nil, err
		}
		batch.expect = s.seen
		if s.status != nil {
			batch.entries = append(batch.entries, batchEntry{key: s4wave_prom.StatusKey(), value: s.status})
			batch.status = s.status
			s.status = nil
		}
		batch.maintain = s.hasCommitted && compactBucket(batch.maxTime+1) > compactBucket(s.committedMax+1)
		s.frozen = batch
		s.open = NewWindow(s.table.Len())
	}

	// Mark the commit running and hand out the batch.
	s.inFlight = true
	return s.frozen, nil
}

// finish ends the in-flight commit of batch. A written batch becomes
// committed state. A failed batch stays frozen for the next commit.
func (s *Store) finish(batch *commitBatch, written bool) {
	// End the commit. A failed commit leaves the frozen batch for the
	// next try.
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.inFlight = false
	if !written {
		return
	}

	// Move the committed state past the written batch.
	s.frozen = nil
	s.committedSeries = batch.endSeries
	s.committedMax, s.hasCommitted = batch.maxTime, true
	if batch.status != nil {
		s.seen = batch.status
	}
}

// standDown ends the in-flight commit without writing it and wakes scrape waiters.
func (s *Store) standDown() {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	s.inFlight = false
	if !s.isLost {
		s.isLost = true
		close(s.lost)
	}
}

// write stores batch in the target's tree within wtx. Writing a batch again
// stores the same keys with the same values, so a replayed or retried commit
// is harmless. It returns ErrStoodDown, writing nothing, when the target's
// status is not the one the Store last saw.
func (s *Store) write(ctx context.Context, wtx world.WorldState, batch *commitBatch) error {
	// Open the target. It must exist because a Store is created or opened
	// on it.
	obj, err := world.MustGetObject(ctx, wtx, s.key)
	if err != nil {
		return errors.Wrapf(err, "target %q", s.key)
	}
	defer world.ReleaseObjectState(obj)

	// Set the batch's keys in the target's tree and move the object root.
	_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		// Open the tree for writing.
		kv, err := block_kvtx.BuildKvTransaction(ctx, bcs, true)
		if err != nil {
			return err
		}
		defer kv.Discard()

		// Stand down when another writer has written the status since.
		status, _, err := kv.Get(ctx, s4wave_prom.StatusKey())
		if err != nil {
			return err
		}
		if !bytes.Equal(status, batch.expect) {
			return ErrStoodDown
		}

		// Set every entry and maintain the segments when the batch opens a
		// bucket, then commit the tree to move the object root.
		for _, e := range batch.entries {
			if err := kv.Set(ctx, e.key, e.value); err != nil {
				return err
			}
		}
		if batch.maintain {
			if err := maintain(ctx, kv, batch.maxTime, s.retention); err != nil {
				return errors.Wrap(err, "maintain segments")
			}
		}
		return kv.Commit(ctx)
	})
	return err
}

// ExportLabels renames each label of lset that the target also sets to
// exported_<name>, as Prometheus does without honor_labels.
func (s *Store) ExportLabels(lset labels.Labels) labels.Labels {
	// Keep the set when no name clashes.
	meta := s.meta
	clash := false
	lset.Range(func(l labels.Label) { clash = clash || meta.Has(l.Name) })
	if !clash {
		return lset
	}

	// Rebuild it with the clashing names prefixed.
	b := labels.NewScratchBuilder(lset.Len())
	lset.Range(func(l labels.Label) {
		name := l.Name
		if meta.Has(name) {
			name = "exported_" + name
		}
		b.Add(name, l.Value)
	})
	b.Sort()
	return b.Labels()
}
