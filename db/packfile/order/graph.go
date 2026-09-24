package order

import (
	"context"
	"slices"

	"github.com/s4wave/spacewave/db/block"
)

// Graph orders a pack's available blocks by their structural relationships.
// Child order is supplied by the producer; unavailable children are ignored.
// It holds references, not block data, and is not concurrency safe.
type Graph struct {
	// nodes contains the available blocks and their ordered outgoing edges.
	nodes map[string]graphNode
}

// graphNode retains one block's identity and intrinsic child order.
type graphNode struct {
	// ref identifies the available block.
	ref *block.BlockRef
	// children lists child identities in producer-supplied order.
	children []string
}

// NewGraph constructs an empty pack ordering graph.
func NewGraph() *Graph {
	return &Graph{nodes: make(map[string]graphNode)}
}

// Add records a block and replaces its outgoing edges with the supplied order.
// Empty references are ignored. The graph owns copies of retained references.
func (g *Graph) Add(ref *block.BlockRef, children []*block.BlockRef) {
	// Retain only addressable blocks.
	key := refKey(ref)
	if key == "" {
		return
	}
	if g.nodes == nil {
		g.nodes = make(map[string]graphNode)
	}

	// Preserve child order independently of later caller mutations.
	node := graphNode{ref: ref.CloneVT()}
	for _, child := range children {
		if childKey := refKey(child); childKey != "" {
			node.children = append(node.children, childKey)
		}
	}
	g.nodes[key] = node
}

// Order emits each available block once in depth-first child order. Explicit
// roots take precedence, followed by roots in the available graph. Unconnected
// roots and remaining cycles use stable hash order. Missing external parents
// do not prevent their available subtrees from being grouped.
func (g *Graph) Order(ctx context.Context, roots []*block.BlockRef) ([]*block.BlockRef, error) {
	// Find roots within the available graph, without loading absent blocks.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(g.nodes))
	referenced := make(map[string]struct{}, len(g.nodes))
	for key, node := range g.nodes {
		keys = append(keys, key)
		for _, child := range node.children {
			referenced[child] = struct{}{}
		}
	}
	slices.Sort(keys)

	// Visit explicit roots, then natural roots, then any cyclic components.
	starts := make([]string, 0, len(roots)+len(keys))
	for _, root := range roots {
		starts = append(starts, refKey(root))
	}
	for _, key := range keys {
		if _, ok := referenced[key]; !ok {
			starts = append(starts, key)
		}
	}
	starts = append(starts, keys...)

	// An explicit stack bounds call depth and suppresses shared descendants.
	seen := make(map[string]struct{}, len(g.nodes))
	out := make([]*block.BlockRef, 0, len(g.nodes))
	var stack []string
	for _, start := range starts {
		stack = append(stack, start)
		for len(stack) != 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			key := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if _, ok := seen[key]; ok {
				continue
			}
			node, ok := g.nodes[key]
			if !ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, node.ref.CloneVT())

			// Push in reverse so each subtree retains the supplied child order.
			for _, v := range slices.Backward(node.children) {
				stack = append(stack, v)
			}
		}
	}
	return out, nil
}
