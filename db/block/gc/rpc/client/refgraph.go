package block_gc_rpc_client

import (
	"context"
	"sync"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_gc_rpc "github.com/s4wave/spacewave/db/block/gc/rpc"
)

// RefGraph implements RefGraphOps backed by a RefGraph RPC service.
type RefGraph struct {
	client block_gc_rpc.SRPCRefGraphClient
	mu     sync.Mutex
}

// NewRefGraph constructs a new RefGraph RPC client.
func NewRefGraph(client block_gc_rpc.SRPCRefGraphClient) *RefGraph {
	return &RefGraph{client: client}
}

// AddRef adds a gc/ref edge from subject to object.
func (r *RefGraph) AddRef(ctx context.Context, subject, object string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.addRef(ctx, subject, object)
}

func (r *RefGraph) addRef(ctx context.Context, subject, object string) error {
	resp, err := r.client.AddRef(ctx, &block_gc_rpc.AddRefRequest{
		Subject: subject,
		Object:  object,
	})
	if err != nil {
		return err
	}
	if errStr := resp.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// RemoveRef removes a single gc/ref edge from subject to object.
func (r *RefGraph) RemoveRef(ctx context.Context, subject, object string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.removeRef(ctx, subject, object)
}

func (r *RefGraph) removeRef(ctx context.Context, subject, object string) error {
	resp, err := r.client.RemoveRef(ctx, &block_gc_rpc.RemoveRefRequest{
		Subject: subject,
		Object:  object,
	})
	if err != nil {
		return err
	}
	if errStr := resp.GetError(); errStr != "" {
		return errors.New(errStr)
	}
	return nil
}

// RemoveNodeRefs removes all outgoing gc/ref edges for a node.
func (r *RefGraph) RemoveNodeRefs(ctx context.Context, node string, markOrphaned bool) ([]string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resp, err := r.client.RemoveNodeRefs(ctx, &block_gc_rpc.RemoveNodeRefsRequest{
		Node:         node,
		MarkOrphaned: markOrphaned,
	})
	if err != nil {
		return nil, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, errors.New(errStr)
	}
	return resp.GetTargets(), nil
}

// HasIncomingRefs checks if a node has any incoming gc/ref edges.
func (r *RefGraph) HasIncomingRefs(ctx context.Context, node string) (bool, error) {
	resp, err := r.client.HasIncomingRefs(ctx, &block_gc_rpc.HasIncomingRefsRequest{
		Node: node,
	})
	if err != nil {
		return false, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return false, errors.New(errStr)
	}
	return resp.GetHasRefs(), nil
}

// HasIncomingRefsExcluding checks if a node has any incoming gc/ref edges
// excluding edges from "unreferenced" and the specified source nodes.
func (r *RefGraph) HasIncomingRefsExcluding(
	ctx context.Context,
	node string,
	excluded ...string,
) (bool, error) {
	sources, err := r.GetIncomingRefs(ctx, node)
	if err != nil {
		return false, err
	}
	excludedSet := make(map[string]struct{}, len(excluded)+1)
	excludedSet[block_gc.NodeUnreferenced] = struct{}{}
	for _, src := range excluded {
		excludedSet[src] = struct{}{}
	}
	for _, src := range sources {
		if _, ok := excludedSet[src]; !ok {
			return true, nil
		}
	}
	return false, nil
}

// GetOutgoingRefs returns all targets of gc/ref edges from a node.
func (r *RefGraph) GetOutgoingRefs(ctx context.Context, node string) ([]string, error) {
	resp, err := r.client.GetOutgoingRefs(ctx, &block_gc_rpc.GetOutgoingRefsRequest{
		Node: node,
	})
	if err != nil {
		return nil, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, errors.New(errStr)
	}
	return resp.GetTargets(), nil
}

// GetIncomingRefs returns all sources with gc/ref edges to a node.
func (r *RefGraph) GetIncomingRefs(ctx context.Context, node string) ([]string, error) {
	resp, err := r.client.GetIncomingRefs(ctx, &block_gc_rpc.GetIncomingRefsRequest{
		Node: node,
	})
	if err != nil {
		return nil, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, errors.New(errStr)
	}
	return resp.GetSources(), nil
}

// GetUnreferencedNodes returns all nodes linked from "unreferenced".
func (r *RefGraph) GetUnreferencedNodes(ctx context.Context) ([]string, error) {
	resp, err := r.client.GetUnreferencedNodes(ctx, &block_gc_rpc.GetUnreferencedNodesRequest{})
	if err != nil {
		return nil, err
	}
	if errStr := resp.GetError(); errStr != "" {
		return nil, errors.New(errStr)
	}
	return resp.GetNodes(), nil
}

// AddBlockRef adds gc/ref from source block to target block.
func (r *RefGraph) AddBlockRef(ctx context.Context, source, target *block.BlockRef) error {
	s := block_gc.BlockIRI(source)
	t := block_gc.BlockIRI(target)
	if s == "" || t == "" {
		return nil
	}
	return r.AddRef(ctx, s, t)
}

// AddObjectRoot adds gc/ref from object:{key} to block.
func (r *RefGraph) AddObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	t := block_gc.BlockIRI(ref)
	if t == "" {
		return nil
	}
	return r.AddRef(ctx, block_gc.ObjectIRI(objectKey), t)
}

// RemoveObjectRoot removes gc/ref from object:{key} to block.
func (r *RefGraph) RemoveObjectRoot(ctx context.Context, objectKey string, ref *block.BlockRef) error {
	t := block_gc.BlockIRI(ref)
	if t == "" {
		return nil
	}
	return r.RemoveRef(ctx, block_gc.ObjectIRI(objectKey), t)
}

// ApplyRefBatch applies a batch of ref graph edge additions and removals.
// The RPC protocol has no batch endpoint, so the client serializes the full
// ownership transition and performs orphan marking before issuing the calls.
func (r *RefGraph) ApplyRefBatch(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	owners := make(map[string]map[string]struct{})
	stagingRemoves := make(map[string]struct{})
	for _, edge := range removes {
		if edge.Subject == block_gc.NodeUnreferenced {
			stagingRemoves[edge.Object] = struct{}{}
			continue
		}
		if block_gc.IsPermanentRoot(edge.Object) {
			continue
		}
		if _, ok := owners[edge.Object]; ok {
			continue
		}
		sources, err := r.GetIncomingRefs(ctx, edge.Object)
		if err != nil {
			return err
		}
		set := make(map[string]struct{}, len(sources))
		for _, source := range sources {
			if source != block_gc.NodeUnreferenced {
				set[source] = struct{}{}
			}
		}
		owners[edge.Object] = set
	}
	for _, edge := range removes {
		if set, ok := owners[edge.Object]; ok {
			delete(set, edge.Subject)
		}
	}
	for _, edge := range adds {
		if set, ok := owners[edge.Object]; ok && edge.Subject != block_gc.NodeUnreferenced {
			set[edge.Subject] = struct{}{}
		}
	}
	for object, set := range owners {
		if len(set) == 0 {
			if _, removingStaging := stagingRemoves[object]; !removingStaging {
				adds = append(adds, block_gc.RefEdge{
					Subject: block_gc.NodeUnreferenced,
					Object:  object,
				})
			}
		}
	}
	for _, e := range adds {
		if err := r.addRef(ctx, e.Subject, e.Object); err != nil {
			return err
		}
	}
	for _, e := range removes {
		if err := r.removeRef(ctx, e.Subject, e.Object); err != nil {
			return err
		}
	}
	return nil
}

// Close is a no-op for the RPC client.
func (r *RefGraph) Close() error {
	return nil
}

// _ is a type assertion
var _ block_gc.RefGraphOps = ((*RefGraph)(nil))
