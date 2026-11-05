package kvtx

import (
	"context"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
)

// livenessPrefix holds the Volume's liveness records, as block_gc.Records
// lays them out. Store formats 2 to 6 kept their reference graph here; format
// 7 keeps its child lists in place and writes the rest of liveness beside
// them.
var livenessPrefix = []byte{2}

// OpenRecords returns the liveness records of the Volume store whose
// transaction is tx.
func OpenRecords(tx kvtx.TxOps) *block_gc.Records {
	return block_gc.NewRecords(tx, livenessPrefix)
}

// readRecords runs fn over the liveness records in one read snapshot.
func (v *Volume) readRecords(ctx context.Context, fn func(rec *block_gc.Records) error) error {
	tx, err := v.kvtxStore.NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	return fn(block_gc.NewRecords(tx, livenessPrefix))
}

// GetChildren returns the child list of a stored block and whether it is
// known. A Volume that keeps no liveness knows none.
func (v *Volume) GetChildren(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, bool, error) {
	// A Volume without liveness knows no lists.
	if v == nil || !v.SupportsRootRetention() {
		return nil, false, nil
	}

	// Read the list, an empty one for a leaf.
	var children []*block.BlockRef
	var found bool
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		var err error
		children, found, err = rec.GetChildren(ctx, ref)
		return err
	})
	if found && children == nil {
		children = []*block.BlockRef{}
	}
	return children, found, err
}

// HasChildLists reports, for each ref, whether the Volume keeps its child
// list, which is whether the Volume stores it in the liveness sense: a block
// whose bytes were evicted keeps its list. It reads all of them in one
// snapshot.
func (v *Volume) HasChildLists(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	listed := make([]bool, len(refs))
	if !v.SupportsRootRetention() {
		return listed, nil
	}
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		for i, ref := range refs {
			var err error
			if listed[i], err = rec.HasChildren(ctx, ref); err != nil {
				return err
			}
		}
		return nil
	})
	return listed, err
}

// GetBucketRoot returns the block the bucket's root name holds, or nil when
// the name is free.
func (v *Volume) GetBucketRoot(ctx context.Context, bucketID, name string) (*block.BlockRef, error) {
	var ref *block.BlockRef
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		entry, err := rec.GetRoot(ctx, bucketID, name)
		if err != nil || entry == nil {
			return err
		}
		ref, _, err = block_gc.ParseBlockKey(entry.GetRef())
		return err
	})
	return ref, err
}

// ListBucketRoots calls fn with each root of the bucket whose name starts with
// prefix, in name order, from one snapshot. fn runs outside the snapshot.
func (v *Volume) ListBucketRoots(ctx context.Context, bucketID, prefix string, fn func(block.NamedRoot) error) error {
	// Read the roots in one snapshot.
	var roots []block.NamedRoot
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		return rec.ListRoots(ctx, bucketID, prefix, func(name string, entry *block_gc.RootEntry) error {
			ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
			if err != nil {
				return err
			}
			roots = append(roots, block.NamedRoot{Name: name, Ref: ref, Expires: unixMilliTime(entry.GetExpiresUnixMilli())})
			return nil
		})
	})
	if err != nil {
		return err
	}

	// Call fn with each.
	for _, root := range roots {
		if err := fn(root); err != nil {
			return err
		}
	}
	return nil
}

// SetBucketRoots stores entries and moves the bucket's named roots in one
// physical transaction, as block_gc.Land describes: it writes nothing and
// returns block.ErrRootMoved when a root does not hold its Previous. Entries
// return volume.ErrReserve while the volume is in the reserve state; moving
// roots alone writes no new data and continues.
func (v *Volume) SetBucketRoots(ctx context.Context, bucketID string, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	// A Volume without atomic commits holds no roots, and refuses new data
	// below the disk reserve.
	if !v.SupportsRootRetention() {
		return block.SetRootsUnheld(ctx, v, entries, roots)
	}
	if len(entries) != 0 && !onlyTombstones(entries) {
		if err := v.checkReserve(ctx, batchBytes(entries)); err != nil {
			return err
		}
	}

	// Store the entries and move the roots in one transaction.
	v.noteUnknownRefsBatch(putRoots, entries)
	var existed []bool
	err := v.withDirectAtomic(ctx, func(p *physicalTx) error {
		var err error
		existed, err = v.land(ctx, p, bucketID, entries, roots)
		return err
	})

	// Count the blocks the committed entries stored, or the refusal.
	if err == nil {
		v.countPutBatch(putRoots, entries, existed)
	}
	v.countRefused(putRoots, err)
	return err
}

// ReleaseBucket releases every named root of bucketID, so the collector frees
// what only they reached, such as the blocks of a deleted Space. It releases
// the roots it read in one transaction; a root a writer moved meanwhile is
// read and released again.
func (v *Volume) ReleaseBucket(ctx context.Context, bucketID string) error {
	for {
		// Read the bucket's roots.
		var release []block.NamedRoot
		err := v.ListBucketRoots(ctx, bucketID, "", func(root block.NamedRoot) error {
			release = append(release, block.NamedRoot{Name: root.Name, Previous: root.Ref})
			return nil
		})
		if err != nil || len(release) == 0 {
			return err
		}

		// Release them, again if one moved between the read and the write.
		err = v.SetBucketRoots(ctx, bucketID, nil, release)
		if !isRootMoved(err) {
			return err
		}
	}
}

// ReleaseBucketPrefix releases every named root of each bucket whose ID
// starts with prefix, one bucket per transaction.
func (v *Volume) ReleaseBucketPrefix(ctx context.Context, prefix string) error {
	// Find the buckets under the prefix.
	buckets := make(map[string]struct{})
	err := v.readRecords(ctx, func(rec *block_gc.Records) error {
		return rec.ScanRoots(ctx, func(bucketID, _ string, _ *block_gc.RootEntry) error {
			if strings.HasPrefix(bucketID, prefix) {
				buckets[bucketID] = struct{}{}
			}
			return nil
		})
	})
	if err != nil {
		return err
	}

	// Release each.
	for bucketID := range buckets {
		if err := v.ReleaseBucket(ctx, bucketID); err != nil {
			return err
		}
	}
	return nil
}

// WalkBucket calls fn with each block a named root of bucketID reaches
// through child lists, once each, from one snapshot. The walk keeps the
// visited blocks in memory, one key each, so it suits a move or an audit, not
// the request path.
func (v *Volume) WalkBucket(ctx context.Context, bucketID string, fn func(*block.BlockRef) error) error {
	return v.readRecords(ctx, func(rec *block_gc.Records) error {
		// Seed the walk with the bucket's roots.
		var stack []*block.BlockRef
		err := rec.ListRoots(ctx, bucketID, "", func(_ string, entry *block_gc.RootEntry) error {
			ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
			if err == nil {
				stack = append(stack, ref)
			}
			return err
		})
		if err != nil {
			return err
		}

		// Visit each reached block once.
		seen := make(map[string]struct{})
		for len(stack) != 0 {
			ref := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			key := string(block_gc.BlockKey(ref))
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			if err := fn(ref); err != nil {
				return err
			}
			children, _, err := rec.GetChildren(ctx, ref)
			if err != nil {
				return err
			}
			stack = append(stack, children...)
		}
		return nil
	})
}

// GetRoot returns the block the Volume's own root name holds, as
// GetBucketRoot does for the bucket with the empty ID.
func (v *Volume) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return v.GetBucketRoot(ctx, "", name)
}

// ListRoots lists the Volume's own roots, as ListBucketRoots does for the
// bucket with the empty ID.
func (v *Volume) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return v.ListBucketRoots(ctx, "", prefix, fn)
}

// SetRoots moves the Volume's own roots, as SetBucketRoots does for the bucket
// with the empty ID. A Volume that cannot commit blocks and roots atomically
// holds no roots: it stores the entries and never frees by liveness.
func (v *Volume) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	if !v.SupportsRootRetention() {
		return block.SetRootsUnheld(ctx, v, entries, roots)
	}
	return v.SetBucketRoots(ctx, "", entries, roots)
}

// heldRoot returns the block the root named name of bucketID holds in p, or
// nil.
func heldRoot(ctx context.Context, p *physicalTx, bucketID, name string) (*block.BlockRef, error) {
	entry, err := p.rec.GetRoot(ctx, bucketID, name)
	if err != nil || entry == nil {
		return nil, err
	}
	ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
	return ref, err
}
