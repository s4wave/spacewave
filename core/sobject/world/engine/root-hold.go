package sobject_world_engine

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// rootHold gathers the blocks and named roots of a local store that one call
// applies together, so holding several roots costs one volume transaction. A
// store without root ownership gathers nothing.
type rootHold struct {
	// store is the local block store that holds the roots.
	store block.StoreOps
	// skip is set when store has no root ownership.
	skip bool
	// entries are the blocks the named roots hold.
	entries []*block.PutBatchEntry
	// roots are the named roots to move.
	roots []block.NamedRoot
}

// newRootHold returns an empty hold on store.
func newRootHold(store block.StoreOps) *rootHold {
	return &rootHold{store: store, skip: !block.SupportsRootRetention(store)}
}

// world holds the World graph under root under name, or releases name when
// root is empty. It stores only the root block, read locally or fetched on
// demand: the volume keeps every block this device wrote or read that the
// root still reaches, and reads fetch the rest on demand.
func (h *rootHold) world(ctx context.Context, name string, root *block.BlockRef) error {
	// Gather nothing for a store without root ownership.
	if h.skip {
		return nil
	}

	// Release the name of an empty World.
	if root.GetEmpty() {
		h.release(name)
		return nil
	}

	// Store the root block with its refs.
	stored, err := h.store.GetStoredBlock(ctx, root)
	if err != nil {
		return err
	}
	if stored == nil {
		return block.ErrNotFound
	}
	h.entries = append(h.entries, &block.PutBatchEntry{Ref: root, Data: stored.GetData(), Refs: stored.GetRefs()})
	h.roots = append(h.roots, block.NamedRoot{Name: name, Ref: root})
	return nil
}

// refBlock holds data as a block whose outgoing refs are refs under name, so
// the store keeps every block refs reach.
func (h *rootHold) refBlock(name string, data []byte, refs []*block.BlockRef) error {
	// Gather nothing for a store without root ownership.
	if h.skip {
		return nil
	}

	// Address the block and hold it.
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return err
	}
	h.entries = append(h.entries, &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs})
	h.roots = append(h.roots, block.NamedRoot{Name: name, Ref: ref})
	return nil
}

// rootSet holds the World graph of each root under name, or releases name
// when roots is empty. Like world, it copies nothing: the set block references
// the roots, and the volume keeps their blocks this device holds.
func (h *rootHold) rootSet(name string, roots []*RetainedRoot) error {
	// Release the set when it is empty.
	if len(roots) == 0 {
		h.release(name)
		return nil
	}

	// Hold the set block referencing the roots.
	refs := make([]*block.BlockRef, 0, len(roots))
	for _, root := range roots {
		refs = append(refs, root.GetRootRef())
	}
	data, err := (&RetainedRootSet{Roots: roots}).MarshalVT()
	if err != nil {
		return err
	}
	return h.refBlock(name, data, refs)
}

// release releases name.
func (h *rootHold) release(name string) {
	if !h.skip {
		h.roots = append(h.roots, block.NamedRoot{Name: name})
	}
}

// apply writes the gathered blocks and moves every gathered root at once.
func (h *rootHold) apply(ctx context.Context) error {
	return block.SetRetainedRoots(ctx, h.store, h.entries, h.roots)
}
