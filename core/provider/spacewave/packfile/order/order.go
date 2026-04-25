// Package order contains packfile block ordering helpers.
package order

import (
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
)

const objectIRIPrefix = "object:"

// RefGraph is the GC graph surface required for pack locality ordering.
type RefGraph interface {
	// GetOutgoingRefs returns all gc/ref targets from a node.
	GetOutgoingRefs(ctx context.Context, node string) ([]string, error)
	// GetIncomingRefs returns all gc/ref sources for a node.
	GetIncomingRefs(ctx context.Context, node string) ([]string, error)
}

// BlockRefs orders refs by walking GC object roots and then appending remaining
// refs in stable hash order.
func BlockRefs(ctx context.Context, graph RefGraph, refs []*block.BlockRef) ([]*block.BlockRef, error) {
	candidates := make(map[string]*block.BlockRef, len(refs))
	for _, ref := range refs {
		key := refKey(ref)
		if key == "" {
			continue
		}
		if _, ok := candidates[key]; !ok {
			candidates[key] = ref
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}

	keys := sortedKeys(candidates)
	if graph == nil {
		return refsForKeys(keys, candidates), nil
	}

	roots, err := objectRootedRefs(ctx, graph, keys, candidates)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(candidates))
	ordered := make([]*block.BlockRef, 0, len(candidates))
	for _, root := range roots {
		if err := visitRef(ctx, graph, root.ref, candidates, seen, &ordered); err != nil {
			return nil, err
		}
	}
	for _, key := range keys {
		if _, ok := seen[key]; ok {
			continue
		}
		ordered = append(ordered, candidates[key])
	}
	return ordered, nil
}

type rootedRef struct {
	root string
	key  string
	ref  *block.BlockRef
}

func objectRootedRefs(
	ctx context.Context,
	graph RefGraph,
	keys []string,
	candidates map[string]*block.BlockRef,
) ([]rootedRef, error) {
	var roots []rootedRef
	for _, key := range keys {
		ref := candidates[key]
		incoming, err := graph.GetIncomingRefs(ctx, block_gc.BlockIRI(ref))
		if err != nil {
			return nil, errors.Wrap(err, "get incoming gc refs")
		}
		for _, source := range incoming {
			if !strings.HasPrefix(source, objectIRIPrefix) {
				continue
			}
			roots = append(roots, rootedRef{
				root: source,
				key:  key,
				ref:  ref,
			})
		}
	}
	slices.SortFunc(roots, func(a, b rootedRef) int {
		if a.root < b.root {
			return -1
		}
		if a.root > b.root {
			return 1
		}
		if a.key < b.key {
			return -1
		}
		if a.key > b.key {
			return 1
		}
		return 0
	})
	return roots, nil
}

func visitRef(
	ctx context.Context,
	graph RefGraph,
	ref *block.BlockRef,
	candidates map[string]*block.BlockRef,
	seen map[string]struct{},
	ordered *[]*block.BlockRef,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	key := refKey(ref)
	if key == "" {
		return nil
	}
	if _, ok := candidates[key]; !ok {
		return nil
	}
	if _, ok := seen[key]; ok {
		return nil
	}
	seen[key] = struct{}{}
	*ordered = append(*ordered, candidates[key])

	outgoing, err := graph.GetOutgoingRefs(ctx, block_gc.BlockIRI(ref))
	if err != nil {
		return errors.Wrap(err, "get outgoing gc refs")
	}
	slices.Sort(outgoing)
	for _, iri := range outgoing {
		child, ok := block_gc.ParseBlockIRI(iri)
		if !ok {
			continue
		}
		if err := visitRef(ctx, graph, child, candidates, seen, ordered); err != nil {
			return err
		}
	}
	return nil
}

func sortedKeys(candidates map[string]*block.BlockRef) []string {
	keys := make([]string, 0, len(candidates))
	for key := range candidates {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

func refsForKeys(keys []string, candidates map[string]*block.BlockRef) []*block.BlockRef {
	out := make([]*block.BlockRef, 0, len(keys))
	for _, key := range keys {
		out = append(out, candidates[key])
	}
	return out
}

func refKey(ref *block.BlockRef) string {
	if ref == nil || ref.GetEmpty() {
		return ""
	}
	h := ref.GetHash()
	if h == nil {
		return ""
	}
	return h.MarshalString()
}
