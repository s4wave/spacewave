package block_gc

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// FrontRoot is the named root a walk first reached a front entry from.
type FrontRoot struct {
	// BucketID is the bucket of the root.
	BucketID string
	// Name is the name of the root.
	Name string
}

// FrontRoots walks from each named root in turn through the stored child
// lists, verified or not, in one snapshot, and then calls visit with each
// front entry in key order and the first root that reached it, or nil for an
// entry no root reaches. Such an entry is held only by blocks no root
// reaches: a parked block or one below it, which the sweep deletes after the
// flip, or a block whose own holder's unverified list is cut short. A lease
// that lapsed still counts as a root, since its root holds its blocks until
// the collector drops it.
//
// It returns the number of named roots and the number of blocks with a child
// list that they reach, which are the stored blocks and the evicted ones; a
// listed block they do not reach is one a collector with frees on would
// free.
//
// It keeps the reached set in memory, one block key per reached block, and
// reads each reached block's list once, so it suits a copy host or an audit,
// not the request path.
func (r *ListRebuild) FrontRoots(ctx context.Context, visit func(ref *block.BlockRef, root *FrontRoot) error) (roots int, listed uint64, err error) {
	// Read the front, the roots and the reach of each root in one snapshot.
	var keys [][]byte
	var names []FrontRoot
	from := make(map[string]int)
	err = r.store.View(ctx, func(rec *Records) error {
		// Read the front's keys.
		keys, names, listed = keys[:0], names[:0], 0
		clear(from)
		err := rec.scan(ctx, rec.key(kindFront), nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
			keys = append(keys, bytes.Clone(key))
			from[string(key)] = -1
			return true, nil
		})
		if err != nil {
			return err
		}

		// Read the named roots.
		var heads []*block.BlockRef
		err = rec.ScanRoots(ctx, func(bucketID, name string, entry *RootEntry) error {
			// Keep the root's name beside the block its walk starts at.
			ref, _, err := ParseBlockKey(entry.GetRef())
			if err != nil {
				return err
			}
			names = append(names, FrontRoot{BucketID: bucketID, Name: name})
			heads = append(heads, ref)
			return nil
		})
		if err != nil {
			return err
		}

		// Walk from each root, depth first, visiting each block once, and
		// note the root that first reaches each front entry.
		seen := make(map[string]struct{})
		var stack []*block.BlockRef
		for i, head := range heads {
			stack = append(stack[:0], head)
			for len(stack) != 0 {
				// Take the next block this walk has not visited.
				ref := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				key := string(BlockKey(ref))
				if _, ok := seen[key]; ok {
					continue
				}
				seen[key] = struct{}{}
				if err := ctx.Err(); err != nil {
					return err
				}
				if root, ok := from[key]; ok && root < 0 {
					from[key] = i
				}

				// Follow its stored list.
				children, has, err := rec.GetChildren(ctx, ref)
				if err != nil {
					return err
				}
				if has {
					listed++
				}
				stack = append(stack, children...)
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, err
	}

	// Visit each front entry outside the view.
	for _, key := range keys {
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return 0, 0, err
		}
		var root *FrontRoot
		if i := from[string(key)]; i >= 0 {
			root = &names[i]
		}
		if err := visit(ref, root); err != nil {
			return 0, 0, err
		}
	}
	return len(names), listed, nil
}
