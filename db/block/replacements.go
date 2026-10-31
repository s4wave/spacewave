package block

import (
	"context"
)

// Replacements lists the stored pages the writes a ReplacementLog recorded
// stopped naming in one interval, so the caller can learn what died without
// walking the graph.
//
// It assumes each page has one parent in the old tree. A page with a second
// parent, such as a block put twice or a subtree copied by reference, may be
// listed while the other parent still names it.
type Replacements struct {
	// Replaced lists dead pages whose own children are accounted here: a stored
	// page a write encoded again, or a page the interval created and dropped.
	Replaced []*BlockRef
	// Removed lists the roots of dead stored subtrees. Each page under one is
	// dead too, except below a page in Replaced or Kept.
	Removed []*BlockRef
	// Kept lists live pages a write attached under a new parent, such as a
	// moved subtree or a written page. Expanding Removed stops at them.
	Kept []*BlockRef
	// Held lists the stored pages, among Kept, that may have gained a holder
	// without being put: a page the interval wrote, or the new root, now names
	// a stored page that no page the interval replaced named, such as another
	// World's object root or a copied subtree. A page put again in the interval
	// had its holders counted at the put instead. Each adds one shared count
	// when the record is named. A held page whose old parent the record lists dead moved instead,
	// as when the root moves onto a page below it; applying the record takes
	// that count back.
	Held []*BlockRef
	// Existed lists the live pages whose edge from the new tree was counted
	// already: a put found the page stored, or only found or untouched pages
	// name it. When a listed parent of one frees, the page loses that
	// parent's count. The old root is among them when the new tree reaches
	// it through an existing page, and then loses the lineage's count.
	Existed []*BlockRef
}

// Expand visits each page in Removed and each page under one, stopping at the
// pages in Replaced and Kept, reading child lists from src.
//
// Returns the errors of WalkGraph, such as ErrNotFound at a hole.
func (r *Replacements) Expand(ctx context.Context, src StoreOps, visit func(ref *BlockRef) error) error {
	stop := make(map[string]struct{}, len(r.Replaced)+len(r.Kept))
	for _, ref := range r.Replaced {
		stop[ref.MarshalString()] = struct{}{}
	}
	for _, ref := range r.Kept {
		stop[ref.MarshalString()] = struct{}{}
	}
	return WalkGraph(ctx, src, r.Removed, 0, func(ref *BlockRef, _ *StoredBlock) error {
		if _, ok := stop[ref.MarshalString()]; ok {
			return ErrSkipRefs
		}
		return visit(ref)
	})
}
