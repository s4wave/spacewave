package block_gc

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// RootReach is what one walk from every named root through the stored child
// lists found: the roots, the first of them to reach each block, and the
// front as the walk's snapshot held it.
type RootReach struct {
	// names is the named roots in the order the walk took them.
	names []FrontRoot
	// front is the front's keys in key order.
	front [][]byte
	// first is the index in names of the first root to reach each block the
	// walk visited, by block key.
	first map[string]int
	// listed is the number of visited blocks with a child list.
	listed uint64
}

// WalkRoots walks from each named root in turn through the stored child
// lists, verified or not, and reads the front, all in one snapshot. A lease
// that lapsed still counts as a root, since its root holds its blocks until
// the collector drops it.
//
// It keeps the reached set in memory, one block key per reached block, and
// reads each reached block's list once, so it suits a copy host or an audit,
// not the request path.
func (r *ListRebuild) WalkRoots(ctx context.Context) (*RootReach, error) {
	// Read the front and the reach of each root in one snapshot.
	var reach *RootReach
	err := r.store.View(ctx, func(rec *Records) error {
		// Read the front's keys.
		reach = &RootReach{first: make(map[string]int)}
		err := rec.scan(ctx, rec.key(kindFront), nil, func(key []byte, _ func() ([]byte, error)) (bool, error) {
			reach.front = append(reach.front, bytes.Clone(key))
			return true, nil
		})
		if err != nil {
			return err
		}

		// Note the root that first reaches each block.
		return rec.reach(ctx, reach)
	})
	if err != nil {
		return nil, err
	}
	return reach, nil
}

// Roots returns the number of named roots the walk started from.
func (x *RootReach) Roots() int {
	return len(x.names)
}

// Listed returns the number of blocks with a child list that the roots
// reach, which are the stored blocks and the evicted ones. A listed block
// the roots do not reach is one a collector with frees on would free.
func (x *RootReach) Listed() uint64 {
	return x.listed
}

// Root returns the first named root whose walk reached ref, or nil for a
// block no root reaches, which a collector with frees on would free. So it
// answers whether a block that something references outside a child list,
// as a weak ref does, is held by another path.
func (x *RootReach) Root(ref *block.BlockRef) *FrontRoot {
	return x.root(BlockKey(ref))
}

// root returns the first named root whose walk reached the block of key.
func (x *RootReach) root(key []byte) *FrontRoot {
	i, ok := x.first[string(key)]
	if !ok {
		return nil
	}
	return &x.names[i]
}

// EachFront calls visit with each front entry in key order and the first
// root that reaches it, or nil for an entry no root reaches. Such an entry
// is held only by blocks no root reaches: a parked block or one below it,
// which the sweep deletes after the flip, or a block whose own holder's
// unverified list is cut short.
func (x *RootReach) EachFront(visit func(ref *block.BlockRef, root *FrontRoot) error) error {
	for _, key := range x.front {
		ref, _, err := ParseBlockKey(key)
		if err != nil {
			return err
		}
		if err := visit(ref, x.root(key)); err != nil {
			return err
		}
	}
	return nil
}

// reach reads the named roots and walks from each in turn through the stored
// child lists, depth first, visiting each block once. It fills into with the
// roots, the first root to reach each block, and the number of visited
// blocks with a child list.
func (r *Records) reach(ctx context.Context, into *RootReach) error {
	// Read the named roots.
	var heads []*block.BlockRef
	err := r.ScanRoots(ctx, func(bucketID, name string, entry *RootEntry) error {
		// Keep the root's name beside the block its walk starts at.
		ref, _, err := ParseBlockKey(entry.GetRef())
		if err != nil {
			return err
		}
		into.names = append(into.names, FrontRoot{BucketID: bucketID, Name: name})
		heads = append(heads, ref)
		return nil
	})
	if err != nil {
		return err
	}

	// Walk from each root, and note the root that first reaches each block.
	var stack []*block.BlockRef
	for i, head := range heads {
		stack = append(stack[:0], head)
		for len(stack) != 0 {
			// Take the next block no walk has visited.
			ref := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			key := string(BlockKey(ref))
			if _, ok := into.first[key]; ok {
				continue
			}
			into.first[key] = i
			if err := ctx.Err(); err != nil {
				return err
			}

			// Follow its stored list.
			children, has, err := r.GetChildren(ctx, ref)
			if err != nil {
				return err
			}
			if has {
				into.listed++
			}
			stack = append(stack, children...)
		}
	}
	return nil
}
