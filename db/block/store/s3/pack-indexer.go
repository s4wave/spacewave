//go:build !tinygo

package block_store_s3

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile"
	"golang.org/x/sync/errgroup"
)

const (
	// indexPackPause is the time the background indexer rests after a pack.
	indexPackPause = 50 * time.Millisecond
	// indexBatchPause is the time it rests after a write transaction, which
	// lets the writers of the same KV store in.
	indexBatchPause = 10 * time.Millisecond
)

// IndexProgress counts the work of indexing packs.
type IndexProgress struct {
	// Packs is the packs indexed.
	Packs uint64
	// Keys is the block keys written.
	Keys uint64
	// Bytes is the pack bytes read.
	Bytes uint64
}

// runIndexer indexes the listed packs that the index has not covered, one at a
// time and slowly, until ctx ends. It holds no lock while it reads or writes,
// so no read waits for it. A restart resumes from the packs whose p/ row is not
// indexed.
func (s *PackStore) runIndexer(ctx context.Context) error {
	for {
		var progress IndexProgress
		if err := s.indexPending(ctx, 1, indexPackPause, indexBatchPause, &progress); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return context.Canceled
		case <-s.index.work:
		}
	}
}

// indexPending indexes every pack the index lists as unindexed, width packs at
// a time, resting pack after each pack and batch after each write transaction.
// Progress accumulates in progress, which other goroutines may read with
// atomic loads.
func (s *PackStore) indexPending(ctx context.Context, width int, pack, batch time.Duration, progress *IndexProgress) error {
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(max(width, 1))
	for _, id := range s.index.pendingIDs() {
		eg.Go(func() error {
			entry := s.index.pendingEntry(id)
			if entry == nil {
				return nil
			}
			keys, size, err := s.indexPack(ctx, entry, batch)
			if errors.Is(err, ErrNotFound) {
				// The next listing drops the entry.
				return nil
			}
			if err != nil {
				return errors.Wrap(err, "index packfile "+id)
			}
			atomic.AddUint64(&progress.Packs, 1)
			atomic.AddUint64(&progress.Keys, keys)
			atomic.AddUint64(&progress.Bytes, size)
			return sleep(ctx, pack)
		})
	}
	return eg.Wait()
}

// indexPack reads the index tail of a pack backwards from the end of the
// object, and writes its block keys to the index. It returns the keys written
// and the bytes read.
func (s *PackStore) indexPack(ctx context.Context, entry *packfile.PackfileEntry, batch time.Duration) (uint64, uint64, error) {
	// Read the index tail, counting the bytes fetched.
	id := entry.GetId()
	size := entry.GetSizeBytes()
	var read uint64
	tail, err := packfile.ReadIndexTail(ctx, size, func(ctx context.Context, off int64, length int) ([]byte, error) {
		read += uint64(length) //nolint:gosec // length is a nonnegative read length.
		return s.client.GetObjectRange(ctx, s.bucket, s.prefix+packDir+id, off, length)
	})
	if err != nil {
		return 0, read, err
	}

	// Write the tail's block keys to the index in paced batches.
	keys, err := s.index.writeKeys(ctx, id, func(yield func([]byte) error) error {
		return packfile.ScanIndexTail(tail, size, func(ie *kvfile.IndexEntry) error {
			return yield(ie.GetKey())
		})
	}, func(ctx context.Context) error { return sleep(ctx, batch) })
	return keys, read, err
}

// sleep waits d or until ctx ends.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// BuildIndex lists the packs, reads the entries the index does not hold, and
// indexes every pack, width packs at a time without pausing, for a store
// that is not serving reads. report is called about once a second with the
// progress so far. The store must have been opened with an Index.
func (s *PackStore) BuildIndex(ctx context.Context, width int, report func(IndexProgress)) error {
	if s.index == nil {
		return errors.New("pack store has no index")
	}
	if err := s.loadEntries(ctx); err != nil {
		return err
	}
	if s.listings.Load() == 0 {
		if err := s.listEntries(ctx, 0); err != nil {
			return err
		}
	}
	var progress IndexProgress
	done := make(chan struct{})
	defer close(done)
	if report != nil {
		go func() {
			t := time.NewTicker(time.Second)
			defer t.Stop()
			for {
				select {
				case <-done:
					return
				case <-t.C:
					report(progress.load())
				}
			}
		}()
	}
	if err := s.indexPending(ctx, width, 0, 0, &progress); err != nil {
		return err
	}
	if report != nil {
		report(progress.load())
	}
	return nil
}

func (p *IndexProgress) load() IndexProgress {
	return IndexProgress{
		Packs: atomic.LoadUint64(&p.Packs),
		Keys:  atomic.LoadUint64(&p.Keys),
		Bytes: atomic.LoadUint64(&p.Bytes),
	}
}
