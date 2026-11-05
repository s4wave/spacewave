package sobject_world_engine

import (
	"context"
	fmt "fmt"

	"github.com/pkg/errors"
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
	// recount asks the store to recount its shared counts before it moves
	// the roots, as AtomicPublication.Recount does.
	recount bool
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
		return fmt.Errorf("replay hold root %s: %w", name, block.NotFoundError(root))
	}
	if !stored.RefsKnown {
		return errors.Wrap(block.ErrRefsUnknown, root.MarshalString())
	}
	h.entries = append(h.entries, &block.PutBatchEntry{Ref: root, Data: stored.Data, Refs: stored.Refs, RefsKnown: true})
	h.roots = append(h.roots, block.NamedRoot{Name: name, Ref: root})
	return nil
}

// worlds holds the World graphs under roots under name in one reference
// block, or releases name when roots is empty. It carries every kept page the
// roots reach, so the store then holds each graph whole, and stores each other
// root block as world does.
func (h *rootHold) worlds(ctx context.Context, name string, pages *worldPages, roots []*block.BlockRef) error {
	// Gather nothing for a store without root ownership.
	if h.skip {
		return nil
	}
	if len(roots) == 0 {
		h.release(name)
		return nil
	}

	// Carry the kept pages, then the root blocks not among them.
	entries := pages.pagesOf(roots...)
	kept := make(map[string]struct{}, len(entries))
	for _, e := range entries {
		kept[e.Ref.MarshalString()] = struct{}{}
	}
	for _, root := range roots {
		if _, ok := kept[root.MarshalString()]; ok {
			continue
		}
		stored, err := h.store.GetStoredBlock(ctx, root)
		if err != nil {
			return err
		}
		if stored == nil {
			return fmt.Errorf("replay hold root %s: %w", name, block.NotFoundError(root))
		}
		if !stored.RefsKnown {
			return errors.Wrap(block.ErrRefsUnknown, root.MarshalString())
		}
		entries = append(entries, &block.PutBatchEntry{Ref: root, Data: stored.Data, Refs: stored.Refs, RefsKnown: true})
	}
	h.entries = append(h.entries, entries...)

	// Name them from one block.
	data, err := (&ReplaySpan{Worlds: roots}).MarshalVT()
	if err != nil {
		return err
	}
	return h.refBlock(ctx, name, data, roots)
}

// spans holds the span chains at spans under name, or releases name when
// spans is empty. A chain the store lacks needs nothing held.
func (h *rootHold) spans(ctx context.Context, name string, spans []*block.BlockRef) error {
	if len(spans) == 0 {
		h.release(name)
		return nil
	}
	data, err := (&ReplaySpan{Payloads: spans}).MarshalVT()
	if err != nil {
		return err
	}
	return h.refBlock(ctx, name, data, spans)
}

// refBlock holds data as a block whose outgoing refs are refs under name, so
// the store keeps every block refs reach.
func (h *rootHold) refBlock(ctx context.Context, name string, data []byte, refs []*block.BlockRef) error {
	// Gather nothing for a store without root ownership.
	if h.skip {
		return nil
	}

	// Address the block and hold it with the stored blocks it names.
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		return err
	}
	h.entries = append(h.entries, &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true})
	h.roots = append(h.roots, block.NamedRoot{Name: name, Ref: ref})
	return nil
}

// rootSet holds the World graph of each root under name, or releases name
// when roots is empty. Like world, it copies nothing: the set block references
// the roots and carries the root blocks the store holds, and the volume keeps
// their blocks this device holds.
func (h *rootHold) rootSet(ctx context.Context, name string, roots []*RetainedRoot) error {
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
	return h.refBlock(ctx, name, data, refs)
}

// release releases name.
func (h *rootHold) release(name string) {
	if !h.skip {
		h.roots = append(h.roots, block.NamedRoot{Name: name})
	}
}

// add gathers the blocks and roots of o.
func (h *rootHold) add(o *rootHold) {
	h.entries = append(h.entries, o.entries...)
	h.roots = append(h.roots, o.roots...)
}

// apply writes the gathered blocks and moves every gathered root at once.
func (h *rootHold) apply(ctx context.Context) error {
	return block.OverwriteRoots(ctx, h.store, h.entries, h.roots)
}
