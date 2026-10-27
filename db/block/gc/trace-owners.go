package block_gc

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// OwnerTrace is the path TraceOwners found from an owner down to a block.
type OwnerTrace struct {
	// Block is the block traced.
	Block string
	// Owner is the node that is not a block, or NodeGCRoot for a root, at the
	// top of the path, empty when no owner was found within the depth.
	Owner string
	// Path lists the blocks from the one Owner holds down to Block.
	Path []string
}

// TraceOwners finds, for each block in blocks, one path up the child lists to
// a node that is not a block. Block edges are stored only forward, so each
// level scans every child list once; maxDepth bounds the levels. It explains
// what keeps a block live, such as one a block pass reached without a child
// list, and is meant for diagnosis, not for a pass.
func (g *RefGraph) TraceOwners(ctx context.Context, blocks []string, maxDepth int) ([]OwnerTrace, error) {
	// up records the blocks whose lists name each block seen; seen holds
	// every block already in a frontier.
	up := make(map[string][]string)
	seen := make(map[string]struct{}, len(blocks))
	owner := make(map[string]string)
	var frontier []string
	for _, block := range blocks {
		if _, ok := seen[block]; !ok && isBlockNode(block) {
			seen[block] = struct{}{}
			frontier = append(frontier, block)
		}
	}

	// Each level reads in its own transaction, so no snapshot is held for
	// the whole trace.
	for depth := 0; depth <= maxDepth && len(frontier) != 0; depth++ {
		err := g.transaction(ctx, false, func(ctx context.Context, tx kvtx.Tx) error {
			// Note the owners and roots holding the frontier directly.
			waiting := make(map[string]struct{}, len(frontier))
			for _, block := range frontier {
				if ok, err := tx.Exists(ctx, graphKey(kindRoot, block)); err != nil {
					return err
				} else if ok {
					owner[block] = NodeGCRoot
					continue
				}
				prefix := graphKey(kindIncoming, block)
				err := tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
					if _, found := owner[block]; !found {
						owner[block] = lastNode(prefix, key)
					}
					return nil
				})
				if err != nil {
					return err
				}
				if _, found := owner[block]; !found {
					waiting[block] = struct{}{}
				}
			}
			if len(waiting) == 0 || depth == maxDepth {
				frontier = nil
				return nil
			}

			// Scan the child lists once for the parents of the rest.
			var next []string
			lists := graphKey(kindChildren)
			err := tx.ScanPrefix(ctx, lists, func(key, value []byte) error {
				listed := prefixBlock + string(key[len(lists):])
				children, err := decodeChildren(value)
				if err != nil {
					return err
				}
				for _, child := range children {
					if _, ok := waiting[child]; !ok {
						continue
					}
					up[child] = append(up[child], listed)
					if _, ok := seen[listed]; !ok && len(seen) < maxTraceBlocks {
						seen[listed] = struct{}{}
						next = append(next, listed)
					}
				}
				return nil
			})
			if err != nil {
				return err
			}
			frontier = next
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// Find a path from each traced block up to an owned block, preferring
	// the shortest, since a block can be named by unreached parents too.
	var traces []OwnerTrace
	for _, block := range blocks {
		if !isBlockNode(block) {
			continue
		}
		trace := OwnerTrace{Block: block}
		from := map[string]string{block: ""}
		queue := []string{block}
		for len(queue) != 0 && trace.Owner == "" {
			cur := queue[0]
			queue = queue[1:]
			if own, ok := owner[cur]; ok {
				trace.Owner = own
				for n := cur; n != ""; n = from[n] {
					trace.Path = append(trace.Path, n)
				}
				break
			}
			for _, p := range up[cur] {
				if _, ok := from[p]; !ok {
					from[p] = cur
					queue = append(queue, p)
				}
			}
		}
		traces = append(traces, trace)
	}
	return traces, nil
}

// maxTraceBlocks bounds the blocks TraceOwners holds while it walks up.
const maxTraceBlocks = 1 << 20
