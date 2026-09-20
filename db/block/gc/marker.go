package block_gc

import (
	"context"

	"github.com/pkg/errors"
)

// Color represents the tri-color state of a node during marking.
type Color int

const (
	// White is the default state. Nodes still white after marking are
	// unreachable and will be swept.
	White Color = iota
	// Grey means the node is known reachable but its outgoing edges have
	// not been scanned yet.
	Grey
	// Black means the node is reachable and fully scanned.
	Black
)

// CollectorGraph is the interface required by the marker and sweep for
// graph traversal. It extends RefGraphOps with node inventory, root set,
// and cleanup operations.
type CollectorGraph interface {
	RefGraphOps
	// IterateNodes returns all node IRIs in the node inventory.
	IterateNodes(ctx context.Context) ([]string, error)
	// GetRootNodes returns all node IRIs in the root set.
	GetRootNodes(ctx context.Context) ([]string, error)
	// RemoveRoot removes a node from the root set.
	RemoveRoot(ctx context.Context, iri string) error
	// RemoveNode removes a node from the node inventory.
	RemoveNode(ctx context.Context, iri string) error
}

// Marker performs an in-memory tri-color mark traversal over a CollectorGraph.
type Marker struct {
	graph CollectorGraph
}

// NewMarker creates a Marker for the given graph backend.
func NewMarker(graph CollectorGraph) *Marker {
	return &Marker{graph: graph}
}

// Mark runs the tri-color mark phase and returns the set of white (unreachable)
// nodes as sweep candidates. The colors map contains the final state of all
// nodes for inspection.
func (m *Marker) Mark(ctx context.Context) (sweepCandidates []string, colors map[string]Color, err error) {
	// Initialize all nodes to white.
	nodes, err := m.graph.IterateNodes(ctx)
	if err != nil {
		return nil, nil, err
	}
	colors = make(map[string]Color, len(nodes))
	for _, n := range nodes {
		colors[n] = White
	}

	// Seed grey set from root nodes.
	roots, err := m.graph.GetRootNodes(ctx)
	if err != nil {
		return nil, nil, err
	}
	grey := make([]string, 0, len(roots))
	for _, r := range roots {
		if colors[r] != Grey {
			colors[r] = Grey
			grey = append(grey, r)
		}
	}

	// Permanent roots retain their descendants even with incomplete inventory.
	for _, pr := range []string{NodeGCRoot, NodeUnreferenced} {
		if colors[pr] == White {
			colors[pr] = Grey
			grey = append(grey, pr)
		}
	}

	// Process grey queue.
	for len(grey) > 0 {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}

		node := grey[len(grey)-1]
		grey = grey[:len(grey)-1]

		// Staging marks identify garbage candidates, not ownership.
		if node == NodeUnreferenced {
			colors[node] = Black
			continue
		}

		targets, err := m.graph.GetOutgoingRefs(ctx, node)
		if err != nil {
			return nil, nil, errors.Wrapf(err, "read outgoing references of %q", node)
		}
		for _, t := range targets {
			c := colors[t]
			if c == White {
				colors[t] = Grey
				grey = append(grey, t)
			}
		}
		colors[node] = Black
	}

	// Collect remaining white nodes.
	for _, n := range nodes {
		if colors[n] == White {
			sweepCandidates = append(sweepCandidates, n)
		}
	}
	return sweepCandidates, colors, nil
}
