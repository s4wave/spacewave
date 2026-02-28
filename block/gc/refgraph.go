// Package block_gc implements a unified reference graph for garbage collection.
//
// The graph uses a single predicate ("gc/ref") for all edges. The meaning of
// an edge from subject to object is: "subject alive implies object should not
// be collected." Nodes are identified by string IRIs that can represent blocks,
// entities, or permanent roots.
//
// Two permanent root IRIs exist: "gcroot" (top of the hierarchy) and
// "unreferenced" (staging area for orphaned nodes). These are never collected.
// When a node loses all incoming gc/ref edges (excluding edges from
// "unreferenced"), its outgoing gc/ref edges are removed and it is linked
// from "unreferenced".
package block_gc

import (
	"context"
	"io"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/cayley/query/shape"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/kvtx"
	kvtx_cayley "github.com/aperturerobotics/hydra/kvtx/cayley"
	kvtx_prefixer "github.com/aperturerobotics/hydra/kvtx/prefixer"
	"github.com/pkg/errors"
)

// RefGraph is a unified reference graph for garbage collection backed by Cayley.
type RefGraph struct {
	handle *cayley.Handle
}

// NewRefGraph constructs a RefGraph backed by the given kvtx store.
// prefix is prepended to all keys (e.g., "gc/" for space context).
func NewRefGraph(ctx context.Context, store kvtx.Store, prefix []byte) (*RefGraph, error) {
	prefixed := kvtx_prefixer.NewPrefixer(store, prefix)
	opts := graph.Options{
		"ignore_duplicate": true,
		"ignore_missing":   true,
	}
	h, err := kvtx_cayley.NewGraph(ctx, prefixed, opts)
	if err != nil {
		return nil, errors.Wrap(err, "new ref graph")
	}
	return &RefGraph{handle: h}, nil
}

// AddRef adds a gc/ref edge from subject to object. Idempotent.
func (rg *RefGraph) AddRef(ctx context.Context, subject, object string) error {
	q := quad.Make(quad.IRI(subject), quad.IRI(PredGCRef), quad.IRI(object), nil)
	return rg.handle.AddQuad(ctx, q)
}

// RemoveRef removes a single gc/ref edge from subject to object.
// Removing a non-existent edge is a no-op.
func (rg *RefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	q := quad.Make(quad.IRI(subject), quad.IRI(PredGCRef), quad.IRI(object), nil)
	return rg.handle.RemoveQuad(ctx, q)
}

// RemoveNodeRefs removes ALL outgoing gc/ref edges for a node.
// Returns the list of target IRIs that lost an incoming edge.
// If markOrphaned is true, targets that have no remaining incoming
// refs (excluding from "unreferenced") get an unreferenced edge.
func (rg *RefGraph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error) {
	targets, err := rg.GetOutgoingRefs(ctx, node)
	if err != nil {
		return nil, err
	}
	for _, t := range targets {
		if err := rg.RemoveRef(ctx, node, t); err != nil {
			return nil, err
		}
	}
	if markOrphaned {
		for _, t := range targets {
			if IsPermanentRoot(t) {
				continue
			}
			has, err := rg.HasIncomingRefs(ctx, t)
			if err != nil {
				return nil, err
			}
			if !has {
				if err := rg.AddRef(ctx, NodeUnreferenced, t); err != nil {
					return nil, err
				}
			}
		}
	}
	return targets, nil
}

// HasIncomingRefs checks if a node has any incoming gc/ref edges.
// Excludes edges from "unreferenced" (those don't count as real refs).
func (rg *RefGraph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	var found bool
	err := filterIterateQuads(ctx, rg.handle, quad.Quad{
		Predicate: quad.IRI(PredGCRef),
		Object:    quad.IRI(node),
	}, func(q quad.Quad) error {
		subj := iriString(q.Subject)
		if subj != NodeUnreferenced {
			found = true
			return io.EOF
		}
		return nil
	})
	if err == io.EOF {
		err = nil
	}
	return found, err
}

// GetOutgoingRefs returns all targets of gc/ref edges from the given node.
func (rg *RefGraph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	var targets []string
	err := filterIterateQuads(ctx, rg.handle, quad.Quad{
		Subject:   quad.IRI(node),
		Predicate: quad.IRI(PredGCRef),
	}, func(q quad.Quad) error {
		targets = append(targets, iriString(q.Object))
		return nil
	})
	return targets, err
}

// GetIncomingRefs returns all sources that have gc/ref edges pointing to the given node.
func (rg *RefGraph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	var sources []string
	err := filterIterateQuads(ctx, rg.handle, quad.Quad{
		Predicate: quad.IRI(PredGCRef),
		Object:    quad.IRI(node),
	}, func(q quad.Quad) error {
		sources = append(sources, iriString(q.Subject))
		return nil
	})
	return sources, err
}

// GetUnreferencedNodes returns all nodes that have a gc/ref from "unreferenced".
func (rg *RefGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	return rg.GetOutgoingRefs(ctx, NodeUnreferenced)
}

// Close closes the underlying graph handle.
func (rg *RefGraph) Close() error {
	return rg.handle.Close()
}

// AddBlockRef adds gc/ref from source block to target block.
func (rg *RefGraph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	s := BlockIRI(source)
	t := BlockIRI(target)
	if s == "" || t == "" {
		return nil
	}
	return rg.AddRef(ctx, s, t)
}

// AddObjectRoot adds gc/ref from object:{key} to block.
func (rg *RefGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	t := BlockIRI(ref)
	if t == "" {
		return nil
	}
	return rg.AddRef(ctx, ObjectIRI(objectKey), t)
}

// RemoveObjectRoot removes gc/ref from object:{key} to block.
func (rg *RefGraph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	t := BlockIRI(ref)
	if t == "" {
		return nil
	}
	return rg.RemoveRef(ctx, ObjectIRI(objectKey), t)
}


// filterIterateQuads iterates over quads matching the input quad.
// Empty fields in the filter quad are ignored (wildcard).
func filterIterateQuads(ctx context.Context, h *cayley.Handle, gq quad.Quad, cb func(q quad.Quad) error) error {
	var q shape.Quads
	if gq.Subject != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Subject, Values: shape.Lookup([]quad.Value{gq.Subject})})
	}
	if gq.Predicate != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Predicate, Values: shape.Lookup([]quad.Value{gq.Predicate})})
	}
	if gq.Object != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Object, Values: shape.Lookup([]quad.Value{gq.Object})})
	}
	if gq.Label != nil {
		q = append(q, shape.QuadFilter{Dir: quad.Label, Values: shape.Lookup([]quad.Value{gq.Label})})
	}

	sh, _, err := shape.Optimize(ctx, q, h)
	if err != nil {
		return err
	}
	it := sh.BuildIterator(ctx, h).Iterate(ctx)
	defer it.Close()
	rsc := graph.NewResultReader(ctx, h, it)
	for {
		rd, err := rsc.ReadQuad(ctx)
		if err == nil {
			err = cb(rd)
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// iriString extracts the string value from a quad.Value, assuming it is an IRI.
func iriString(v quad.Value) string {
	if v == nil {
		return ""
	}
	iri, ok := v.(quad.IRI)
	if ok {
		return string(iri)
	}
	return ""
}
