package space_world

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// worldHistory is a named root that holds every World it was moved onto
// since a checkpoint last covered them, as one HeldRoots block. A kept World
// names stored pages that only an earlier World of the history may hold, so
// moving the root onto the newest World alone would leave those pages held
// by nothing. The history trims a World once no position or candidate the
// adapter still keeps is that World.
type worldHistory struct {
	// name is the named root.
	name string
	// roots are the Worlds the root holds, oldest first. Guarded by the
	// adapter's rootMtx.
	roots []*block.BlockRef
}

// holds reports whether the history holds root. The caller holds rootMtx.
func (h *worldHistory) holds(root *block.BlockRef) bool {
	return slices.ContainsFunc(h.roots, root.EqualsRef)
}

// releases reports whether the history holds a World keep does not report.
// The caller holds rootMtx.
func (h *worldHistory) releases(keep func(*block.BlockRef) bool) bool {
	return slices.ContainsFunc(h.roots, func(r *block.BlockRef) bool { return !keep(r) })
}

// addHistory adds root to h and moves its root onto the history, with
// entries stored in the same change. The caller holds rootMtx.
func (a *Adapter) addHistory(ctx context.Context, h *worldHistory, root *block.BlockRef, entries ...*block.PutBatchEntry) error {
	roots := h.roots
	if !root.GetEmpty() && !h.holds(root) {
		roots = append(slices.Clone(h.roots), root.Clone())
	}
	return a.setHistories(ctx, []*worldHistory{h}, [][]*block.BlockRef{roots}, entries)
}

// held returns the Worlds of h that keep reports, followed by the live
// Worlds it holds in place of the ones it releases. The caller holds
// rootMtx.
func (h *worldHistory) held(keep func(*block.BlockRef) bool, live []*block.BlockRef) []*block.BlockRef {
	roots := slices.DeleteFunc(slices.Clone(h.roots), func(r *block.BlockRef) bool { return !keep(r) })
	for _, root := range live {
		roots = append(roots, root.Clone())
	}
	return dedupRefs(roots)
}

// setHistories moves each history of hs onto the Worlds of the same index of
// next, and roots onto their refs, in one root move with entries, so either
// every move lands or none does. A history whose Worlds stay is not moved.
// The caller holds rootMtx.
func (a *Adapter) setHistories(ctx context.Context, hs []*worldHistory, next [][]*block.BlockRef, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	// Encode each changed history as one block naming its Worlds, or release
	// its name when it holds none.
	for i, h := range hs {
		if slices.EqualFunc(next[i], h.roots, refEqual) {
			continue
		}
		if len(next[i]) == 0 {
			roots = append(roots, block.NamedRoot{Name: h.name})
			continue
		}
		entry, err := refsEntry(&HeldRoots{Roots: next[i]}, next[i])
		if err != nil {
			return err
		}
		entries = append(entries, entry)
		roots = append(roots, block.NamedRoot{Name: h.name, Ref: entry.Ref})
	}

	// Move them together, then record the Worlds each holds.
	if err := a.moveRootsLocked(ctx, entries, roots...); err != nil {
		return err
	}
	for i, h := range hs {
		h.roots = next[i]
	}
	return nil
}

// readHistory reads the Worlds h's root holds. The caller holds rootMtx.
func (a *Adapter) readHistory(ctx context.Context, h *worldHistory) error {
	// Read the block the root holds.
	ref := a.held[h.name]
	if ref.GetEmpty() {
		return nil
	}
	data, found, err := a.pages.GetBlock(ctx, ref)
	if err != nil || !found {
		return err
	}

	// Decode its Worlds.
	msg := &HeldRoots{}
	if err := msg.UnmarshalVT(data); err != nil {
		return err
	}
	h.roots = msg.GetRoots()
	return nil
}
