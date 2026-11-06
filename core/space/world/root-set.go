package space_world

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// rootSet is a named root that holds a list of roots as one HeldRoots block,
// moved only through setRootSets with the block it held as Previous.
//
// The cursor, pending and checkpoint roots are World histories: each holds
// every World it was moved onto since a checkpoint last covered them, since a
// kept World names stored pages that only an earlier World of the history may
// hold, and moving the root onto the newest World alone would leave those
// pages held by nothing. A trim releases a World once no position or candidate
// the adapter still keeps is that World. The replay payloads root holds the
// payloads of the operations above the checkpoint and of the queued ones.
//
// Invariants, which the Volume's collector relies on to free nothing live:
//
//  1. roots is what the stored root's block names: it changes only after the
//     move that stores the new block lands, and it is read from that block at
//     open before any move, so a move after a restart keeps every root the
//     stored block names unless the caller drops it.
//  2. A move names a set's old block as Previous, so one that another writer
//     moved first fails and changes nothing.
//  3. A move's new block lands in the same Land as the release of the old
//     one, so a root both name nets its new holder and its lost one in the
//     same netting slice and is never at zero between them.
//  4. A root a move adds is held until that Land by what put it: the kept
//     pages the swap stores, a replay stage epoch, a pin, or another root.
//  5. A set whose roots stay is not moved; the stored block already holds
//     them. A set that holds none releases its name and forgets its roots,
//     so the next move holds only what its caller names.
//  6. A move back onto a block the set left is a root moved onto a stored
//     block, which sets the Volume's hold mark when that block is at zero, so
//     no free takes a child before the block's re-hold nets.
type rootSet struct {
	// name is the named root.
	name string
	// roots are the roots the named root holds, oldest first. Guarded by the
	// adapter's rootMtx.
	roots []*block.BlockRef
}

// holds reports whether the set holds root. The caller holds rootMtx.
func (h *rootSet) holds(root *block.BlockRef) bool {
	return slices.ContainsFunc(h.roots, root.EqualsRef)
}

// releases reports whether the set holds a root keep does not report. The
// caller holds rootMtx.
func (h *rootSet) releases(keep func(*block.BlockRef) bool) bool {
	return slices.ContainsFunc(h.roots, func(r *block.BlockRef) bool { return !keep(r) })
}

// addRoot adds root to h and moves h's named root onto the new list, with
// entries stored in the same change. The caller holds rootMtx.
func (a *Adapter) addRoot(ctx context.Context, h *rootSet, root *block.BlockRef, entries ...*block.PutBatchEntry) error {
	roots := h.roots
	if !root.GetEmpty() && !h.holds(root) {
		roots = append(slices.Clone(h.roots), root.Clone())
	}
	return a.setRootSets(ctx, []*rootSet{h}, [][]*block.BlockRef{roots}, entries)
}

// held returns the Worlds of h that keep reports, followed by the live
// Worlds it holds in place of the ones it releases. The caller holds
// rootMtx.
func (h *rootSet) held(keep func(*block.BlockRef) bool, live []*block.BlockRef) []*block.BlockRef {
	roots := slices.DeleteFunc(slices.Clone(h.roots), func(r *block.BlockRef) bool { return !keep(r) })
	for _, root := range live {
		roots = append(roots, root.Clone())
	}
	return dedupRefs(roots)
}

// setRootSets moves each set of hs onto the roots of the same index of next,
// and roots onto their refs, in one root move with entries, so either every
// move lands or none does. A set whose roots stay is not moved. The caller
// holds rootMtx.
func (a *Adapter) setRootSets(ctx context.Context, hs []*rootSet, next [][]*block.BlockRef, entries []*block.PutBatchEntry, roots ...block.NamedRoot) error {
	// Encode each changed set as one block naming its roots, or release its
	// name when it holds none.
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

	// Move them together, then record the roots each holds.
	if err := a.moveRootsLocked(ctx, entries, roots...); err != nil {
		return err
	}
	for i, h := range hs {
		h.roots = next[i]
	}
	return nil
}

// readRootSet reads the roots h's named root holds. The caller holds rootMtx.
func (a *Adapter) readRootSet(ctx context.Context, h *rootSet) error {
	// Read the block the root holds.
	ref := a.held[h.name]
	if ref.GetEmpty() {
		return nil
	}
	data, found, err := a.pages.GetBlock(ctx, ref)
	if err != nil || !found {
		return err
	}

	// Decode its roots.
	msg := &HeldRoots{}
	if err := msg.UnmarshalVT(data); err != nil {
		return err
	}
	h.roots = msg.GetRoots()
	return nil
}
