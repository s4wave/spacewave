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

// addHistory adds root to h and moves its root onto the history, with
// entries stored in the same change. The caller holds rootMtx.
func (a *Adapter) addHistory(ctx context.Context, h *worldHistory, root *block.BlockRef, entries ...*block.PutBatchEntry) error {
	if root.GetEmpty() || h.holds(root) {
		if len(entries) == 0 {
			return nil
		}
		return a.moveRootsLocked(ctx, entries)
	}
	return a.setHistory(ctx, h, append(slices.Clone(h.roots), root.Clone()), entries...)
}

// trimHistory keeps only the Worlds of h that keep reports, moving its root
// when the set changes. The caller holds rootMtx.
func (a *Adapter) trimHistory(ctx context.Context, h *worldHistory, keep func(*block.BlockRef) bool) error {
	roots := slices.DeleteFunc(slices.Clone(h.roots), func(r *block.BlockRef) bool { return !keep(r) })
	if len(roots) == len(h.roots) {
		return nil
	}
	return a.setHistory(ctx, h, roots)
}

// setHistory moves h's root onto a block naming roots, or releases it when
// roots is empty. The caller holds rootMtx.
func (a *Adapter) setHistory(ctx context.Context, h *worldHistory, roots []*block.BlockRef, entries ...*block.PutBatchEntry) error {
	// Release the name when the history is empty.
	if len(roots) == 0 {
		if err := a.moveRootsLocked(ctx, entries, block.NamedRoot{Name: h.name}); err != nil {
			return err
		}
		h.roots = nil
		return nil
	}

	// Hold every World under one block.
	entry, err := refsEntry(&HeldRoots{Roots: roots}, roots)
	if err != nil {
		return err
	}
	if err := a.moveRootsLocked(ctx, append(entries, entry), block.NamedRoot{Name: h.name, Ref: entry.Ref}); err != nil {
		return err
	}
	h.roots = roots
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
