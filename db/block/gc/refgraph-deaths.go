package block_gc

import (
	"context"
	"encoding/binary"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// ResolveDeaths applies the shared count rule to the deadlist record of an
// interval that started at the root from, reading the graph in one
// transaction and writing nothing. It calls visit with each page the record
// kills, parent before child, and whether the page frees, and returns the
// change to each shared count the record makes. The bool reports a freed
// parent (including an unknown child list): a shadow caller retaining its bytes
// must rebuild the counts before a later writer trusts those stored edges.
//
// The walk starts at each listed page that no listed page names: a page a write
// dirtied before dropping its parent's subtree is listed replaced under a
// removed parent, and loses its count only through that parent. Each page is
// decided once, after every parent in the walk naming it: it loses one count
// for each such parent that frees, and a root of the walk one more for the
// holder that dropped it, and is alive when its count covers them and frees
// otherwise. A page all of whose walk parents live is alive. Below a page in
// Removed, every page is walked too, down to the pages in Kept or named by a
// replaced page without being listed, which are live. Each live child of a
// killed page then
// changes by its parent's fate: a child whose edge from the new tree was
// counted already, in Held or Existed, loses one when the parent frees; any
// other child gains one when the parent lives, since the new tree's written
// parent names it too. The old root loses the lineage's count last when it is
// in Existed.
//
// A page in Dropped loses its own holder as a root of the walk. Its edges
// were never counted: freeing it takes none from its children, while a page
// another writer adopted must fund each child edge once before a child is
// decided. Counts always cover the edges of a surviving page.
//
// A killed page whose child list the graph does not hold is a hole: the walk
// skips its children, which can only leave a count too high.
func (g *RefGraph) ResolveDeaths(ctx context.Context, from *block.BlockRef, dead *block.Replacements, visit func(node string, frees bool) error) (map[string]int64, bool, error) {
	return g.resolveDeaths(ctx, from, dead, visit, 0)
}

// ErrDeathWalkBudget reports a discarded plan that exceeded its work bound.
// Its caller publishes no partial changes or observations.
var ErrDeathWalkBudget = errors.New("death walk exceeded its work bound")

// ResolveDeathsBounded limits the nodes and edges read for one shadow plan.
// A spent bound returns ErrDeathWalkBudget before visiting any fate; a caller
// retaining physical bytes can abandon that history and rebuild its counts.
func (g *RefGraph) ResolveDeathsBounded(ctx context.Context, from *block.BlockRef, dead *block.Replacements, limit int, visit func(node string, frees bool) error) (map[string]int64, bool, error) {
	return g.resolveDeaths(ctx, from, dead, visit, limit)
}

func (g *RefGraph) resolveDeaths(ctx context.Context, from *block.BlockRef, dead *block.Replacements, visit func(node string, frees bool) error, limit int) (map[string]int64, bool, error) {
	// Index the record by node.
	set := func(lists ...[]*block.BlockRef) map[string]struct{} {
		out := make(map[string]struct{})
		for _, list := range lists {
			for _, ref := range list {
				out[BlockIRI(ref)] = struct{}{}
			}
		}
		return out
	}
	listed := set(dead.Replaced, dead.Removed, dead.Dropped)
	dropped := set(dead.Dropped)
	live := set(dead.Kept)
	counted := set(dead.Held, dead.Existed)

	// Walk the record in one read transaction, gathering the count changes.
	changes := make(map[string]int64)
	freedParent := false
	err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
		// Fate decisions require one complete, known accounting basis.
		state, err := readSharingTx(ctx, tx)
		if err != nil {
			return err
		}
		if !state.Phase.Counting() || state.Doubt {
			return ErrSharingWalk
		}
		work := 0
		readChildren := func(node string) ([]string, bool, error) {
			// Charge each decoded list before retaining any of its edges.
			list, known, err := readChildrenTx(ctx, tx, node)
			if err != nil {
				return nil, false, err
			}
			work += 1 + len(list)
			if limit > 0 && work > limit {
				return nil, false, ErrDeathWalkBudget
			}
			return list, known, nil
		}

		// Read the listed pages' child lists: their listed children are not
		// roots of the walk, and a replaced page's unlisted children are live.
		// A dropped page names no child the death walk follows, but its children
		// are needed if another writer adopted the page before this record applies.
		childLists := make(map[string][]string, len(listed))
		knownChildren := make(map[string]bool, len(listed))
		named := make(map[string]struct{})
		for node := range listed {
			children, known, err := readChildren(node)
			if err != nil {
				return err
			}
			childLists[node], knownChildren[node] = children, known
			if _, ok := dropped[node]; ok {
				continue
			}
			for _, child := range children {
				if _, ok := listed[child]; ok {
					named[child] = struct{}{}
				}
			}
		}
		for _, ref := range dead.Replaced {
			for _, child := range childLists[BlockIRI(ref)] {
				if _, ok := listed[child]; !ok {
					live[child] = struct{}{}
				}
			}
		}

		// Gather the walk: each root, and each page below one through
		// children that are not live, with the number of walk parents naming
		// it. A page's child list is read once.
		children := func(node string) ([]string, error) {
			if list, ok := childLists[node]; ok {
				return list, nil
			}
			list, known, err := readChildren(node)
			childLists[node], knownChildren[node] = list, known
			return list, err
		}
		var roots []string
		for node := range listed {
			if _, ok := named[node]; !ok {
				roots = append(roots, node)
			}
		}
		slices.Sort(roots)
		parents := make(map[string]int, len(listed))
		seen := make(map[string]struct{}, len(listed))
		stk := slices.Clone(roots)
		for len(stk) != 0 {
			node := stk[len(stk)-1]
			stk = stk[:len(stk)-1]
			if _, ok := seen[node]; ok {
				continue
			}
			seen[node] = struct{}{}
			if _, ok := dropped[node]; ok {
				continue
			}
			list, err := children(node)
			if err != nil {
				return err
			}
			for _, child := range list {
				if _, isLive := live[child]; !isLive {
					parents[child]++
					stk = append(stk, child)
				}
			}
		}

		// A surviving dropped parent funds its edges before any walked child is
		// decided. These ordering edges do not remove a child's independent root
		// loss, and a freeing dropped parent passes no loss to its children.
		for node := range dropped {
			for _, child := range childLists[node] {
				if _, walked := seen[child]; walked {
					parents[child]++
				}
			}
		}

		// Decide each page once all its walk parents are decided, in node
		// order among the ready pages. A page loses one count for each walk
		// parent that frees, and a root one more for the holder that dropped
		// it; it lives while its count covers them.
		killed := make(map[string]int64, len(roots))
		for _, root := range roots {
			killed[root] = 1
		}
		ready := make([]string, 0, len(roots))
		for _, root := range roots {
			if parents[root] == 0 {
				ready = append(ready, root)
			}
		}
		slices.Reverse(ready)
		for len(ready) != 0 {
			node := ready[len(ready)-1]
			ready = ready[:len(ready)-1]

			// Decide the page's fate.
			lives := true
			if k := killed[node]; k != 0 {
				n, err := readHoldsTx(ctx, tx, node)
				if err != nil {
					return err
				}
				if lives = int64(n)+changes[node] >= k; lives { // #nosec G115 -- counts stay far below 2^63.
					changes[node] -= k
				}
			}
			if !lives && (!knownChildren[node] || len(childLists[node]) != 0) {
				freedParent = true
			}
			if err := visit(node, !lives); err != nil {
				return err
			}

			// Hand each child its parent's fate.
			for _, child := range childLists[node] {
				if _, isDropped := dropped[node]; isDropped {
					if lives {
						changes[child]++
					}
					if _, walked := seen[child]; walked {
						if parents[child]--; parents[child] == 0 {
							ready = append(ready, child)
						}
					}
					continue
				}
				if _, isLive := live[child]; !isLive {
					if !lives {
						killed[child]++
					}
					if parents[child]--; parents[child] == 0 {
						ready = append(ready, child)
					}
					continue
				}
				_, isCounted := counted[child]
				switch {
				case lives && !isCounted:
					changes[child]++
				case !lives && isCounted:
					changes[child]--
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, false, err
	}

	// Take the lineage's count from an old root an existing page still names.
	if !from.GetEmpty() {
		root := BlockIRI(from)
		_, isListed := listed[root]
		if !isListed && slices.ContainsFunc(dead.Existed, func(ref *block.BlockRef) bool { return ref.EqualVT(from) }) {
			changes[root]--
		}
	}
	return changes, freedParent, nil
}

// ChangeHolds adds each change to its block's shared count in one transaction,
// deleting a count that reaches zero. A change that would take a count below
// zero leaves it at zero; it returns how many did, since any means a holder
// was never counted. It refuses with ErrSharingWalk while the counts are not
// complete.
func (g *RefGraph) ChangeHolds(ctx context.Context, changes map[string]int64) (int, error) {
	// Sort the nonzero changes so the writes go in key order.
	if len(changes) == 0 {
		return 0, nil
	}
	nodes := make([]string, 0, len(changes))
	for node, change := range changes {
		if change != 0 {
			nodes = append(nodes, node)
		}
	}
	slices.Sort(nodes)
	var underflows int
	err := g.transaction(ctx, true, func(ctx context.Context, tx kvtx.Tx) error {
		// Refuse unless the counts are complete.
		underflows = 0
		s, err := readSharingTx(ctx, tx)
		if err != nil {
			return err
		}
		if !s.Phase.Counting() || s.Doubt {
			return ErrSharingWalk
		}

		// Change each count.
		for _, node := range nodes {
			if !isBlockNode(node) {
				return errors.Errorf("hold on %q, which is not a block", node)
			}
			n, err := readHoldsTx(ctx, tx, node)
			if err != nil {
				return err
			}
			next := int64(n) + changes[node] // #nosec G115 -- counts stay far below 2^63.
			if next < 0 {
				underflows++
				next = 0
			}
			key := graphKey(kindHold, node)
			if next == 0 {
				err = tx.Delete(ctx, key)
			} else {
				err = tx.Set(ctx, key, binary.AppendUvarint(nil, uint64(next)))
			}
			if err != nil {
				return err
			}
		}
		return nil
	})
	return underflows, err
}
