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

// readBatchReads is the number of range reads a background batch keeps in
// flight, such as the resident warm-up's, so a foreground miss shares the
// bucket with at most this many of them.
const readBatchReads = 2

// readBatchForegroundReads is the number of range reads a foreground batch
// keeps in flight. The account's request gate admits each one; this bounds
// only the batch's goroutines.
const readBatchForegroundReads = 16

// batchExtent is the place of one block of a batch in its pack.
type batchExtent struct {
	// i is the block's index in the batch's refs.
	i int
	// ref is the block.
	ref *block.BlockRef
	// id is the pack holding it.
	id string
	// off and size are its bytes in the pack.
	off  int64
	size int
	// claim is its index in the batch's claim on the shared extent reads.
	claim int
}

// batchRun is one range read of a batch: a span of one pack and the extents
// it holds.
type batchRun struct {
	// id is the pack.
	id string
	// off and end bound the span.
	off, end int64
	// extents are the blocks in the span, by offset.
	extents []batchExtent
}

// GetStoredBlockBatch reads refs as GetStoredBlock reads each, with one range
// read per run of nearby blocks of one pack rather than one per block. A block
// whose index row holds no extent, or whose bytes do not hold it, reads alone
// through GetStoredBlock. A failed range read reaches each block of its run as
// that block's error, as a failed extent read does. A background batch keeps
// readBatchReads range reads in flight, any other readBatchForegroundReads.
// The batch shares its reads with concurrent reads of its blocks, as
// readPlaced describes.
func (s *PackStore) GetStoredBlockBatch(ctx context.Context, refs []*block.BlockRef, fn block.StoredBlockFunc) error {
	// Read every block alone without extents.
	if s.index == nil || !extentReadsOn.Load() {
		return block.GetStoredBlockEach(ctx, s, refs, fn)
	}
	if err := s.Preload(ctx); err != nil {
		return err
	}
	extents, err := s.index.locateExtents(ctx, refs)
	if err != nil {
		return err
	}

	// Read the runs, keeping the blocks they did not hold.
	reads := readBatchForegroundReads
	if block.IsBackground(ctx) {
		reads = readBatchReads
	}
	answered := make([]bool, len(refs))
	err = s.readPlaced(ctx, extents, reads, func(e batchExtent, stored *block.StoredBlock, err error) error {
		if stored == nil && err == nil {
			return nil
		}
		answered[e.i] = true
		return fn(e.i, stored, err)
	})
	if err != nil {
		return err
	}

	// Read the rest alone.
	var rest []*block.BlockRef
	var restIdx []int
	for i, ref := range refs {
		if !answered[i] {
			rest = append(rest, ref)
			restIdx = append(restIdx, i)
		}
	}
	return block.GetStoredBlockEach(ctx, s, rest, func(j int, stored *block.StoredBlock, err error) error {
		return fn(restIdx[j], stored, err)
	})
}

// ReadBatch reads the blocks of refs with as few range reads as their places
// allow, and calls fn with each block read and verified, at most
// readBatchReads range reads at a time. Calls of fn never overlap.
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
	return s.readPlaced(ctx, extents, readBatchReads, func(e batchExtent, stored *block.StoredBlock, err error) error {
		if err != nil {
			return err
		}
		if stored == nil {
			return nil
		}
		return fn(e.ref, stored)
	})
}

// readPlaced reads extents as readRuns does, sharing the reads with
// concurrent readers of the same blocks: a cached copy answers an extent
// without a read, an extent another read is fetching waits for that read, and
// a reader of a block the batch is fetching waits for the batch. Each
// extent's answer reaches fn as readRuns gives it. A shared read that ends
// without reading a block, as when its batch fails, answers neither a block
// nor an error, so the caller reads that block alone.
//
// A foreground reader joining the batch raises every read of the batch to its
// class, so a background batch never holds it behind the background lane.
func (s *PackStore) readPlaced(ctx context.Context, extents []batchExtent, reads int, fn func(batchExtent, *block.StoredBlock, error) error) error {
	// Answer the extents a cached copy holds.
	pending := extents[:0]
	for _, e := range extents {
		if stored := s.cachedBlock(ctx, e.ref, e.id, e.off, e.size); stored != nil {
			if err := fn(e, stored, nil); err != nil {
				return err
			}
			continue
		}
		pending = append(pending, e)
	}

	// Join the reads already in flight, and claim the rest.
	keys := make([]string, len(pending))
	for j := range pending {
		pending[j].claim = j
		keys[j] = string(packfile.BlockKey(pending[j].ref.GetHash()))
	}
	c, rctx := s.extentReads.claim(ctx, keys)
	defer c.release()
	var owned, joined []batchExtent
	for _, e := range pending {
		if c.owned[e.claim] {
			owned = append(owned, e)
		} else {
			joined = append(joined, e)
		}
	}

	// Read the claimed extents, settling each for the readers that joined
	// it. An answer of the batch's own end settles nothing, and release
	// abandons it, so a joiner reads afresh rather than fail with it.
	err := s.readRuns(rctx, owned, reads, func(e batchExtent, stored *block.StoredBlock, err error) error {
		if err != nil && ctx.Err() != nil {
			return context.Cause(ctx)
		}
		c.settle(e.claim, stored, err)
		return fn(e, stored, err)
	})
	if err != nil {
		return err
	}

	// Wait for the joined reads; one its batch abandoned answers nothing.
	for _, e := range joined {
		stored, err := c.wait(ctx, e.claim)
		if errors.Is(err, errFetchAbandoned) {
			stored, err = nil, nil
		}
		if err != nil && ctx.Err() != nil {
			return context.Cause(ctx)
		}
		if err := fn(e, stored, err); err != nil {
			return err
		}
	}
	return nil
}

// readRuns reads extents in runs, as batchRuns merges them, with at most reads
// range reads in flight, and calls fn once for each extent: with its verified
// block, with neither a block nor an error when its pack is missing or its
// bytes do not hold it, or with the run's error when the range read failed.
// Calls of fn never overlap. An error from fn ends the reads.
//
// Runs span gaps up to batchGap of the bucket's prices, so a read carries every
// unwanted byte cheaper than a second request.
func (s *PackStore) readRuns(ctx context.Context, extents []batchExtent, reads int, fn func(batchExtent, *block.StoredBlock, error) error) error {
	// Read the runs, at most reads at a time.
	var mtx sync.Mutex
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(reads)
	for run := range batchRuns(extents, s.batchGap) {
		eg.Go(func() error {
			// Read the span; a missing pack means its rows named the wrong one.
			data, err := s.client.GetObjectRange(egCtx, s.bucket, s.prefix+packDir+run.id, run.off, int(run.end-run.off))
			if errors.Is(err, ErrNotFound) {
				data, err = nil, nil
			}
			if err == nil {
				s.reads.batchReads.Add(1)
				s.reads.downloaded(readBatch, len(data))
			}

			// Pass on each block, as its own copy so a kept block does not
			// pin the span, or the run's error.
			mtx.Lock()
			defer mtx.Unlock()
			for _, e := range run.extents {
				if err != nil {
					if ferr := fn(e, nil, err); ferr != nil {
						return ferr
					}
					continue
				}
				stored := runBlock(data, run.off, e)
				if stored != nil {
					s.reads.batchBlocks.Add(1)
				}
				if ferr := fn(e, stored, nil); ferr != nil {
					return ferr
				}
			}
			return nil
		})
	}
	return eg.Wait()
}

// runBlock returns the verified block of extent e from data, the bytes of a
// run starting at off, or nil when the bytes do not hold it.
func runBlock(data []byte, off int64, e batchExtent) *block.StoredBlock {
	// Cut the block's bytes from the span and check them against its ref.
	start := e.off - off
	if start+int64(e.size) > int64(len(data)) {
		return nil
	}
	stored, err := verifiedBlock(e.ref, slices.Clone(data[start:start+int64(e.size)]))
	if err != nil {
		return nil
	}
	return stored
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
	for i, ref := range refs {
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
			extents = append(extents, batchExtent{i: i, ref: ref, id: id, off: int64(row.off), size: int(row.size)}) // #nosec G115 -- the check above bounds off.
		}
	}
	return extents, nil
}

// _ is a type assertion
var _ block.StoredBlockBatcher = (*PackStore)(nil)
