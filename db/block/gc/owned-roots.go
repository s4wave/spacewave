package block_gc

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// OwnedRoots calls fn with each block an owner outside the block graph holds:
// the first block on every path from NodeGCRoot through nodes that are not
// blocks, such as a bucket, a named root's owner, a reader's pin or a stage,
// each block once with the owner that reached it first. The walk knows no
// kind of owner, so an owner added later is walked like the rest; skip prunes
// a node and everything only it owns, such as another bucket. Every node read
// that fails, and every edge to a node named as a block that does not parse,
// ends the walk with an error, so a caller that cannot read the whole frontier
// learns it instead of seeing too few roots.
//
// It reads one edge list per owner node, so its cost is the owner nodes and
// the blocks they hold directly, not the blocks under them. It returns that
// frontier's size, the owner nodes walked and the blocks reported, which is
// also the entries it held in memory.
func OwnedRoots(ctx context.Context, rg RefGraphOps, skip func(node string) bool, fn func(owner string, ref *block.BlockRef) error) (int, error) {
	// Walk the owner nodes depth first from the root, each once.
	seen := map[string]bool{NodeGCRoot: true}
	reported := make(map[string]bool)
	stack := []string{NodeGCRoot}
	for len(stack) != 0 {
		node := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		targets, err := rg.GetOutgoingRefs(ctx, node)
		if err != nil {
			return 0, errors.Wrap(err, "read the edges of "+FormatIRI(node))
		}

		// Report each block once, and queue each owner not yet seen.
		for _, target := range targets {
			switch {
			case seen[target] || reported[target] || (skip != nil && skip(target)):
			case strings.HasPrefix(target, prefixBlock):
				ref, ok := ParseBlockIRI(target)
				if !ok {
					return 0, errors.Errorf("%s holds a block node that does not parse: %q", FormatIRI(node), target)
				}
				reported[target] = true
				if err := fn(node, ref); err != nil {
					return 0, err
				}
			default:
				seen[target] = true
				stack = append(stack, target)
			}
		}
	}
	return len(seen) + len(reported), nil
}
