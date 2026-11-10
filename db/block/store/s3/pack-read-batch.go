package block_store_s3

import (
	"cmp"
	"context"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"golang.org/x/sync/errgroup"
)

// readBatchLatency and readBatchBandwidth are a range read's fixed cost and
// one read's transfer rate across a network to a bucket. Reading their product
// of unwanted bytes, 500 kB, takes as long as a second request. A bucket on a
// local network has less latency and more bandwidth, and a product near it.
const (
	readBatchLatency   = 20 * time.Millisecond
	readBatchBandwidth = 25e6
)

// batchGap returns the widest gap between two extents of one pack that a batch
// read spans rather than splitting the read: the gap whose bytes cost a second
// request's time, or, on a service that bills egress, its price, whichever is
// less. Amazon S3's prices make the second about 4.8 kB.
func batchGap(p *Pricing) int64 {
	// Time spans up to the bytes one request's latency would carry.
	gap := int64(readBatchLatency.Seconds() * readBatchBandwidth)

	// Price spans up to the bytes whose egress costs one class B request.
	if egress := p.GetEgressGb(); egress > 0 {
		gap = min(gap, int64(p.GetClassBPerMillion()/1e6/egress*gib))
	}
	return gap
}

// readBatchRun bounds the bytes of one range read of a batch, so a read holds
// at most this much in memory and a slow one delays few blocks.
const readBatchRun = 4 << 20

// readBatchReads is the number of range reads a batch keeps in flight. A
// batch is background work, and a foreground miss shares the bucket with it.
const readBatchReads = 2

// batchExtent is the place of one batch ref's block in its pack.
type batchExtent struct {
	// ref is the block.
	ref *block.BlockRef
	// id is the pack holding it.
	id string
	// off and size are its bytes in the pack.
	off  int64
	size int
}

// batchRun is one range read of a batch: a span of one pack and the extents
// inside it.
type batchRun struct {
	// id is the pack.
	id string
	// off and end bound the span.
	off, end int64
	// extents are the blocks in the span, by offset.
	extents []batchExtent
}

// ReadBatch reads the blocks of refs with as few range reads as their places
// allow, and calls fn with each block read and verified. It locates every ref
// in one snapshot of the index, sorts the extents by pack and offset, and
// reads each run of nearby extents of one pack, at most readBatchRun bytes,
// with one range read, readBatchReads at a time. Calls of fn never overlap.
//
// Runs span gaps up to batchGap of the bucket's prices, so a read carries every
// listed block near its neighbours when egress is free.
//
// It reads only the refs whose index rows hold their extents, and skips the
// rest, a missing pack, and bytes that do not hold their block: a caller that
// warms a cache with it reads the skipped refs, if they are wanted, as any
// read does. A failed range read or an error from fn ends the batch with that
// error.
func (s *PackStore) ReadBatch(ctx context.Context, refs []*block.BlockRef, fn func(*block.BlockRef, *block.StoredBlock) error) error {
	// Locate the extents, once the pack table that names their packs loads.
	if s.index == nil || !extentReadsOn.Load() {
		return nil
	}
	if err := s.Preload(ctx); err != nil {
		return err
	}
	extents, err := s.index.locateExtents(ctx, refs)
	if err != nil {
		return err
	}

	// Read the runs, passing on each block in them.
	var mtx sync.Mutex
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(readBatchReads)
	for run := range batchRuns(extents, s.batchGap) {
		eg.Go(func() error {
			// Read the span; a missing pack means its rows named the wrong one.
			data, err := s.client.GetObjectRange(egCtx, s.bucket, s.prefix+packDir+run.id, run.off, int(run.end-run.off))
			if errors.Is(err, ErrNotFound) {
				return nil
			}
			if err != nil {
				return err
			}
			s.reads.batchReads.Add(1)
			s.reads.downloaded(readBatch, len(data))

			// Pass on each block whose bytes hold it, as its own copy, so a
			// kept block does not pin the span.
			mtx.Lock()
			defer mtx.Unlock()
			for _, e := range run.extents {
				start := e.off - run.off
				if start+int64(e.size) > int64(len(data)) {
					continue
				}
				stored, err := verifiedBlock(e.ref, slices.Clone(data[start:start+int64(e.size)]))
				if err != nil {
					continue
				}
				s.reads.batchBlocks.Add(1)
				if err := fn(e.ref, stored); err != nil {
					return err
				}
			}
			return nil
		})
	}
	return eg.Wait()
}

// batchRuns sorts extents by pack and offset and yields its runs: the extents
// of one pack whose gaps are at most gap, spanning at most readBatchRun bytes.
// An extent larger than readBatchRun is a run of its own.
func batchRuns(extents []batchExtent, gap int64) func(yield func(batchRun) bool) {
	return func(yield func(batchRun) bool) {
		slices.SortFunc(extents, func(a, b batchExtent) int {
			return cmp.Or(cmp.Compare(a.id, b.id), cmp.Compare(a.off, b.off))
		})
		var run batchRun
		for i, e := range extents {
			// Extend the run with an extent close enough to its end.
			end := e.off + int64(e.size)
			if i != 0 && e.id == run.id && e.off <= run.end+gap && max(end, run.end)-run.off <= readBatchRun {
				run.end = max(run.end, end)
				run.extents = append(run.extents, e)
				continue
			}

			// Otherwise end the run and start the next at this extent.
			if i != 0 && !yield(run) {
				return
			}
			run = batchRun{id: e.id, off: e.off, end: end, extents: []batchExtent{e}}
		}
		if len(extents) != 0 {
			yield(run)
		}
	}
}

// locateExtents returns the extent of each ref whose h row holds one and
// names a pack the table holds, reading every row in one snapshot. As with
// locateExtent, a pack sharing its number with another may be the wrong one,
// and the caller checks the bytes against the block's hash.
func (x *packIndex) locateExtents(ctx context.Context, refs []*block.BlockRef) ([]batchExtent, error) {
	// Read every row in one snapshot.
	tx, err := x.store.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()

	// Keep the rows that hold an extent in a live pack.
	extents := make([]batchExtent, 0, len(refs))
	for _, ref := range refs {
		if ref.GetHash() == nil {
			continue
		}
		value, found, err := tx.Get(ctx, hashRowKey(packfile.BlockKey(ref.GetHash())))
		if err != nil {
			return nil, err
		}
		row, valid := parseHashValue(value)
		if !found || !valid || !row.extent || row.off > math.MaxInt64-uint64(row.size) {
			continue
		}
		x.mtx.RLock()
		var id string
		if int(row.num) < len(x.ids) {
			id = x.ids[row.num]
		}
		x.mtx.RUnlock()
		if id != "" {
			extents = append(extents, batchExtent{ref: ref, id: id, off: int64(row.off), size: int(row.size)}) // #nosec G115 -- the check above bounds off.
		}
	}
	return extents, nil
}
