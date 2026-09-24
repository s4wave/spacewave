// Package order groups related blocks for physical packfile locality.
package order

import (
	"cmp"
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

// objectIRIPrefix identifies object roots in the GC graph.
const objectIRIPrefix = "object:"

// RefGraph is the GC graph surface required for pack locality ordering.
type RefGraph interface {
	// GetOutgoingRefs returns all gc/ref targets from a node.
	GetOutgoingRefs(ctx context.Context, node string) ([]string, error)
	// GetIncomingRefs returns all gc/ref sources for a node.
	GetIncomingRefs(ctx context.Context, node string) ([]string, error)
}

// rootedRef associates an available block with a named object root.
type rootedRef struct {
	// root identifies the object that retains the block.
	root string
	// key provides a stable tie-breaker within one object.
	key string
	// ref identifies the available root block.
	ref *block.BlockRef
}

// BlockRefs groups refs by available subtrees, preferring named GC object roots.
// GC edges have no intrinsic order, so siblings use stable hash order. With no
// graph, all refs use stable hash order. Only candidate blocks are queried.
func BlockRefs(ctx context.Context, graph RefGraph, refs []*block.BlockRef) ([]*block.BlockRef, error) {
	// Deduplicate candidate identities before querying the graph.
	ordered := NewGraph()
	for _, ref := range refs {
		ordered.Add(ref, nil)
	}
	if graph == nil {
		return ordered.Order(ctx, nil)
	}
	keys := make([]string, 0, len(ordered.nodes))
	for key := range ordered.nodes {
		keys = append(keys, key)
	}
	slices.Sort(keys)

	// Capture relationships even when no object root is present in this upload.
	var roots []rootedRef
	for _, key := range keys {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		ref := ordered.nodes[key].ref
		iri := block_gc.BlockIRI(ref)
		outgoing, err := graph.GetOutgoingRefs(ctx, iri)
		if err != nil {
			return nil, errors.Wrap(err, "get outgoing gc refs")
		}
		slices.Sort(outgoing)
		var children []*block.BlockRef
		for _, target := range outgoing {
			if child, ok := block_gc.ParseBlockIRI(target); ok {
				children = append(children, child)
			}
		}
		ordered.Add(ref, children)

		// Named roots retain their existing object-key precedence.
		incoming, err := graph.GetIncomingRefs(ctx, iri)
		if err != nil {
			return nil, errors.Wrap(err, "get incoming gc refs")
		}
		for _, source := range incoming {
			if strings.HasPrefix(source, objectIRIPrefix) {
				roots = append(roots, rootedRef{root: source, key: key, ref: ref})
			}
		}
	}
	slices.SortFunc(roots, func(a, b rootedRef) int {
		return cmp.Or(cmp.Compare(a.root, b.root), cmp.Compare(a.key, b.key))
	})

	// Use the same structural ordering as producers with intrinsic child order.
	rootRefs := make([]*block.BlockRef, 0, len(roots))
	for _, root := range roots {
		rootRefs = append(rootRefs, root.ref)
	}
	return ordered.Order(ctx, rootRefs)
}

// refKey identifies a nonempty block by its content hash.
func refKey(ref *block.BlockRef) string {
	if ref == nil || ref.GetEmpty() || ref.GetHash() == nil {
		return ""
	}
	return ref.GetHash().MarshalString()
}
