package block_gc

import (
	"context"
	"io"
	"runtime/trace"

	"github.com/aperturerobotics/cayley"
	"github.com/aperturerobotics/cayley/graph"
	cayley_kv "github.com/aperturerobotics/cayley/graph/kv"
	"github.com/aperturerobotics/cayley/graph/refs"
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
		// RefGraph always uses Cayley's default index set; skip reading the
		// index metadata on every world-state rebuild.
		cayley_kv.OptAssumeDefaultIdx: true,
	}
	h, err := kvtx_cayley.NewGraph(ctx, prefixed, opts)
	if err != nil {
		return nil, errors.Wrap(err, "new ref graph")
	}
	return &RefGraph{handle: h}, nil
}

// RegisterEntityChain registers a chain of gc/ref edges between nodes.
// Each adjacent pair gets an AddRef call: nodes[0]->nodes[1],
// nodes[1]->nodes[2], etc. At least 2 nodes required. Idempotent
// (Cayley ignore_duplicate).
func RegisterEntityChain(ctx context.Context, rg RefGraphOps, nodes ...string) error {
	if len(nodes) < 2 {
		return errors.New("RegisterEntityChain requires at least 2 nodes")
	}
	for i := 0; i < len(nodes)-1; i++ {
		if err := rg.AddRef(ctx, nodes[i], nodes[i+1]); err != nil {
			return err
		}
	}
	return nil
}

// AddRef adds a gc/ref edge from subject to object. Idempotent.
func (rg *RefGraph) AddRef(ctx context.Context, subject, object string) error {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/add-ref")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/add-ref/build-quad")
	q := quad.Make(quad.IRI(subject), quad.IRI(PredGCRef), quad.IRI(object), nil)
	subtask.End()

	taskCtx, subtask = trace.NewTask(taskCtx, "hydra/block-gc/refgraph/add-ref/add-quad")
	err := rg.handle.AddQuad(taskCtx, q)
	subtask.End()
	return err
}

// RemoveRef removes a single gc/ref edge from subject to object.
// Removing a non-existent edge is a no-op.
func (rg *RefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	q := quad.Make(quad.IRI(subject), quad.IRI(PredGCRef), quad.IRI(object), nil)
	return rg.handle.RemoveQuad(ctx, q)
}

// ApplyRefBatch applies a batch of ref graph edge additions and removals
// in a single Cayley transaction.
func (rg *RefGraph) ApplyRefBatch(ctx context.Context, adds, removes []RefEdge) error {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/apply-ref-batch")
	defer task.End()

	n := len(adds) + len(removes)
	if n == 0 {
		return nil
	}
	tx := graph.NewTransactionN(n)
	for _, e := range adds {
		tx.AddQuad(quad.Make(quad.IRI(e.Subject), quad.IRI(PredGCRef), quad.IRI(e.Object), nil))
	}
	for _, e := range removes {
		tx.RemoveQuad(quad.Make(quad.IRI(e.Subject), quad.IRI(PredGCRef), quad.IRI(e.Object), nil))
	}
	return rg.handle.ApplyTransaction(ctx, tx)
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
	return rg.HasIncomingRefsExcluding(ctx, node)
}

// HasIncomingRefsExcluding checks if a node has any incoming gc/ref edges.
// Excludes edges from "unreferenced" and the specified source nodes.
func (rg *RefGraph) HasIncomingRefsExcluding(
	ctx context.Context,
	node string,
	excluded ...string,
) (bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding")
	defer task.End()

	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding/resolve-excluded")
	excludedVals := make([]quad.Value, 0, len(excluded)+1)
	excludedVals = append(excludedVals, quad.IRI(NodeUnreferenced))
	for _, src := range excluded {
		excludedVals = append(excludedVals, quad.IRI(src))
	}
	excludedSet := make(map[any]struct{}, len(excludedVals))
	for _, v := range excludedVals {
		ref, err := rg.handle.ValueOf(ctx, v)
		if err != nil {
			return false, err
		}
		if ref == nil {
			continue
		}
		excludedSet[refs.ToKey(ref)] = struct{}{}
	}
	subtask.End()

	var found bool
	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-gc/refgraph/has-incoming-refs-excluding/iterate-candidates")
	err := iterateFilteredNodeRefs(taskCtx, rg.handle, quad.Quad{
		Predicate: quad.IRI(PredGCRef),
		Object:    quad.IRI(node),
	}, quad.Subject, func(ref graph.Ref) error {
		if _, ok := excludedSet[refs.ToKey(ref)]; !ok {
			found = true
			return io.EOF
		}
		return nil
	})
	subtask.End()
	return found, err
}

// GetOutgoingRefs returns all targets of gc/ref edges from the given node.
func (rg *RefGraph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	return collectFilteredNodeIRIs(ctx, rg.handle, quad.Quad{
		Subject:   quad.IRI(node),
		Predicate: quad.IRI(PredGCRef),
	}, quad.Object)
}

// GetIncomingRefs returns all sources that have gc/ref edges pointing to the given node.
func (rg *RefGraph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	ctx, task := trace.NewTask(ctx, "hydra/block-gc/refgraph/get-incoming-refs")
	defer task.End()

	return collectFilteredNodeIRIs(ctx, rg.handle, quad.Quad{
		Predicate: quad.IRI(PredGCRef),
		Object:    quad.IRI(node),
	}, quad.Subject)
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

// buildQuadFilters builds quad filters for the non-empty directions in gq.
func buildQuadFilters(gq quad.Quad) shape.Quads {
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
	return q
}

// iterateFilteredNodeRefs iterates node refs on dir from quads matching gq.
func iterateFilteredNodeRefs(
	ctx context.Context,
	h *cayley.Handle,
	gq quad.Quad,
	dir quad.Direction,
	cb func(ref graph.Ref) error,
) error {
	taskCtx, subtask := trace.NewTask(ctx, "hydra/block-gc/refgraph/iterate-filtered-node-refs/optimize-shape")
	sh, _, err := shape.Optimize(taskCtx, shape.NodesFrom{
		Dir:   dir,
		Quads: buildQuadFilters(gq),
	}, h)
	subtask.End()
	if err != nil {
		return err
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-gc/refgraph/iterate-filtered-node-refs/build-iterator")
	it := sh.BuildIterator(taskCtx, h).Iterate(taskCtx)
	subtask.End()
	defer it.Close()
	taskCtx, subtask = trace.NewTask(ctx, "hydra/block-gc/refgraph/iterate-filtered-node-refs/iterate")
	defer subtask.End()
	for {
		if !it.Next(taskCtx) {
			if err := it.Err(); err != nil {
				return err
			}
			return nil
		}
		ref, err := it.Result(taskCtx)
		if err != nil {
			return err
		}
		if err := cb(ref); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

// collectFilteredNodeIRIs collects node IRIs on dir from quads matching gq.
func collectFilteredNodeIRIs(
	ctx context.Context,
	h *cayley.Handle,
	gq quad.Quad,
	dir quad.Direction,
) ([]string, error) {
	var nodeRefs []graph.Ref
	if err := iterateFilteredNodeRefs(ctx, h, gq, dir, func(ref graph.Ref) error {
		nodeRefs = append(nodeRefs, ref)
		return nil
	}); err != nil {
		return nil, err
	}
	if len(nodeRefs) == 0 {
		return nil, nil
	}
	vals, err := graph.ValuesOf(ctx, h, nodeRefs)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(vals))
	for _, v := range vals {
		out = append(out, iriString(v))
	}
	return out, nil
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
