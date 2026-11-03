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
	// CountsComplete and CountingGeneration certify that all interval puts
	// used the same complete, known funding basis. A small uncertain record
	// uses FallbackHeld; large records recount. Neither replays historical
	// deaths or returns historical From/Found holders.
	CountsComplete bool
	// CountingGeneration identifies the complete Volume count basis used by
	// the interval puts and the first receipt owning its From lineage.
	CountingGeneration uint64
	// FallbackHeld funds every fresh parent's edges without consuming any old
	// holder. It is computed from the puts off the store's writer.
	FallbackHeld []*BlockRef
	// Replaced lists dead stored pages a write encoded again, whose own
	// children are accounted here.
	Replaced []*BlockRef
	// Dropped lists dead pages the interval stored afresh and dropped. No put
	// counted their edges to their children, so applying the record decides
	// each alone and takes no count from the pages it names.
	Dropped []*BlockRef
	// Removed lists the roots of dead stored subtrees. Each page under one is
	// dead too, except below a page in Replaced or Kept.
	Removed []*BlockRef
	// Kept lists live pages a write attached under a new parent, such as a
	// moved subtree or a written page. Expanding Removed stops at them.
	Kept []*BlockRef
	// Held lists the stored pages that gained a holder no put counted, once
	// for each page the interval stored afresh, or the new root, that names
	// one: a page the interval found stored or did not write, such as another
	// World's object root or a copied subtree. Each entry adds one shared
	// count when the record is named. A held page whose
	// old parent the record lists dead moved, as when the root moves onto a
	// page below it; applying the record takes that parent's count back when
	// it frees.
	Held []*BlockRef
	// Existed lists the live pages whose edge from the new tree was counted
	// already: a put found the page stored, or only found or untouched pages
	// name it. When a listed parent of one frees, the page loses that
	// parent's count. The old root is among them when the new tree reaches
	// it through an existing page, and then loses the lineage's count.
	Existed []*BlockRef
	// Found lists each page an interval's put found stored, once per such
	// put. Bucket/stage owners retain its bytes before naming. Ordinary leaf
	// children retain them afterward; only a proven count basis returns the
	// provisional scalar multiplicity after child funding. Uncertainty keeps
	// conservative surplus until recount or physical collection.
	Found []*BlockRef
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

// RefCount counts list occurrences, including repeated provisional holders.
// The count bounds inline record work without scanning the record's pages.
func (r *Replacements) RefCount() int {
	return len(r.Replaced) + len(r.Dropped) + len(r.Removed) + len(r.Kept) + len(r.Held) + len(r.Existed) + len(r.Found) + len(r.FallbackHeld)
}
