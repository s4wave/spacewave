package packmove

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// walkChunk is the nodes one snapshot of the outside walk expands.
const walkChunk = 512

// OutsideWorld is the set of blocks a named root outside the World's block
// store reaches: the account's settings and every other store. The volume's block range holds them with the World's blocks, so
// Pack leaves them alone and Switch keeps their values.
//
// The set errs toward keeping: a block that both the World and another root
// reach is in it, and a block no root reaches is not.
type OutsideWorld struct {
	// keys are the block keys, as BlockRef.MarshalVT encodes them.
	keys map[string]struct{}
	// bytes is the size of the values of the blocks in keys the walk found.
	bytes uint64
}

// Has reports whether the block key, relative to the block prefix, is in the
// set.
func (o *OutsideWorld) Has(key []byte) bool {
	if o == nil {
		return false
	}
	_, ok := o.keys[string(key)]
	return ok
}

// Len returns the number of blocks in the set.
func (o *OutsideWorld) Len() uint64 {
	if o == nil {
		return 0
	}
	return uint64(len(o.keys))
}

// Bytes returns the size of the values of the blocks in the set.
func (o *OutsideWorld) Bytes() uint64 {
	if o == nil {
		return 0
	}
	return o.bytes
}

// outsideWalk is the state of a FindOutsideWorld walk.
type outsideWalk struct {
	// seen holds the blocks already queued.
	seen map[string]struct{}
	// frontier holds the blocks not yet expanded.
	frontier []*block.BlockRef
	// out is the set under construction.
	out *OutsideWorld
}

// queue adds the blocks in refs the walk has not met to the frontier and the
// set.
func (w *outsideWalk) queue(refs []*block.BlockRef) {
	for _, ref := range refs {
		key, err := ref.MarshalVT()
		if err != nil {
			continue
		}
		if _, ok := w.seen[string(key)]; ok {
			continue
		}
		w.seen[string(key)] = struct{}{}
		w.frontier = append(w.frontier, ref)
		w.out.keys[string(key)] = struct{}{}
	}
}

// expandBlocks reads the child list and the size of the value of each block,
// from tx.
func expandBlocks(ctx context.Context, tx kvtx.Tx, keys *store_kvkey.KVKey, refs []*block.BlockRef) (children [][]*block.BlockRef, size uint64, err error) {
	rec := volume_kvtx.OpenRecords(tx)
	for _, ref := range refs {
		list, _, err := rec.GetChildren(ctx, ref)
		if err != nil {
			return nil, 0, errors.Wrapf(err, "read references of %s", ref.MarshalString())
		}
		children = append(children, list)
		key, err := ref.MarshalVT()
		if err != nil {
			return nil, 0, err
		}
		value, _, err := tx.Get(ctx, keys.GetBlockKey(key))
		if err != nil {
			return nil, 0, errors.Wrapf(err, "read block %s", ref.MarshalString())
		}
		size += uint64(len(value))
	}
	return children, size, nil
}

// FindOutsideWorld walks src from every named root outside the bucket
// worldStoreID through child lists, and returns the blocks it reaches. The
// walk reads src in short snapshots, so it runs beside a writer; a block
// another process commits after the walk passed is missing from the set,
// which Switch avoids by walking with the source closed by every other
// process.
func FindOutsideWorld(ctx context.Context, src kvtx.Store, keys *store_kvkey.KVKey, worldStoreID string) (*OutsideWorld, error) {
	// Start from the roots outside the World's bucket.
	w := &outsideWalk{
		seen: make(map[string]struct{}),
		out:  &OutsideWorld{keys: make(map[string]struct{})},
	}
	var roots []*block.BlockRef
	err := ReadStable(ctx, src, func(tx kvtx.Tx) error {
		roots = roots[:0]
		return volume_kvtx.OpenRecords(tx).ScanRoots(ctx, func(bucketID, _ string, entry *block_gc.RootEntry) error {
			// Skip the World's own roots, and take every other.
			if bucketID == worldStoreID {
				return nil
			}
			ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
			if err != nil {
				return err
			}
			roots = append(roots, ref)
			return nil
		})
	})
	if err != nil {
		return nil, errors.Wrap(err, "read the named roots")
	}
	w.queue(roots)

	// Expand a chunk of blocks per snapshot until none are left.
	for len(w.frontier) != 0 {
		at := max(0, len(w.frontier)-walkChunk)
		chunk := w.frontier[at:]
		w.frontier = w.frontier[:at:at]
		var children [][]*block.BlockRef
		var size uint64
		err := ReadStable(ctx, src, func(tx kvtx.Tx) error {
			var err error
			children, size, err = expandBlocks(ctx, tx, keys, chunk)
			return err
		})
		if err != nil {
			return nil, errors.Wrap(err, "walk the child lists")
		}
		w.out.bytes += size
		for _, c := range children {
			w.queue(c)
		}
	}
	return w.out, nil
}
