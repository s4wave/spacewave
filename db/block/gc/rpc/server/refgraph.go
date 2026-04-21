package block_gc_rpc_server

import (
	"context"
	"errors"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_gc_rpc "github.com/s4wave/spacewave/db/block/gc/rpc"
)

// RefGraph implements the RefGraph RPC service.
type RefGraph struct {
	rg block_gc.RefGraphOps
}

// NewRefGraph constructs a new RefGraph RPC server.
func NewRefGraph(rg block_gc.RefGraphOps) *RefGraph {
	return &RefGraph{rg: rg}
}

// AddRef adds a gc/ref edge from subject to object.
func (s *RefGraph) AddRef(
	ctx context.Context,
	req *block_gc_rpc.AddRefRequest,
) (*block_gc_rpc.AddRefResponse, error) {
	if s.rg == nil {
		return nil, errors.ErrUnsupported
	}

	resp := &block_gc_rpc.AddRefResponse{}
	if err := s.rg.AddRef(ctx, req.GetSubject(), req.GetObject()); err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// RemoveRef removes a single gc/ref edge from subject to object.
func (s *RefGraph) RemoveRef(
	ctx context.Context,
	req *block_gc_rpc.RemoveRefRequest,
) (*block_gc_rpc.RemoveRefResponse, error) {
	if s.rg == nil {
		return nil, errors.ErrUnsupported
	}

	resp := &block_gc_rpc.RemoveRefResponse{}
	if err := s.rg.RemoveRef(ctx, req.GetSubject(), req.GetObject()); err != nil {
		resp.Error = err.Error()
	}
	return resp, nil
}

// RemoveNodeRefs removes all outgoing gc/ref edges for a node.
func (s *RefGraph) RemoveNodeRefs(
	ctx context.Context,
	req *block_gc_rpc.RemoveNodeRefsRequest,
) (*block_gc_rpc.RemoveNodeRefsResponse, error) {
	if s.rg == nil {
		return nil, errors.ErrUnsupported
	}

	targets, err := s.rg.RemoveNodeRefs(ctx, req.GetNode(), req.GetMarkOrphaned())
	resp := &block_gc_rpc.RemoveNodeRefsResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Targets = targets
	}
	return resp, nil
}

// HasIncomingRefs checks if a node has any incoming gc/ref edges.
func (s *RefGraph) HasIncomingRefs(
	ctx context.Context,
	req *block_gc_rpc.HasIncomingRefsRequest,
) (*block_gc_rpc.HasIncomingRefsResponse, error) {
	if s.rg == nil {
		return nil, errors.ErrUnsupported
	}

	hasRefs, err := s.rg.HasIncomingRefs(ctx, req.GetNode())
	resp := &block_gc_rpc.HasIncomingRefsResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.HasRefs = hasRefs
	}
	return resp, nil
}

// GetOutgoingRefs returns all targets of gc/ref edges from a node.
func (s *RefGraph) GetOutgoingRefs(
	ctx context.Context,
	req *block_gc_rpc.GetOutgoingRefsRequest,
) (*block_gc_rpc.GetOutgoingRefsResponse, error) {
	if s.rg == nil {
		return nil, errors.ErrUnsupported
	}

	targets, err := s.rg.GetOutgoingRefs(ctx, req.GetNode())
	resp := &block_gc_rpc.GetOutgoingRefsResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Targets = targets
	}
	return resp, nil
}

// GetIncomingRefs returns all sources with gc/ref edges to a node.
func (s *RefGraph) GetIncomingRefs(
	ctx context.Context,
	req *block_gc_rpc.GetIncomingRefsRequest,
) (*block_gc_rpc.GetIncomingRefsResponse, error) {
	if s.rg == nil {
		return nil, errors.ErrUnsupported
	}

	sources, err := s.rg.GetIncomingRefs(ctx, req.GetNode())
	resp := &block_gc_rpc.GetIncomingRefsResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Sources = sources
	}
	return resp, nil
}

// GetUnreferencedNodes returns all nodes linked from "unreferenced".
func (s *RefGraph) GetUnreferencedNodes(
	ctx context.Context,
	req *block_gc_rpc.GetUnreferencedNodesRequest,
) (*block_gc_rpc.GetUnreferencedNodesResponse, error) {
	if s.rg == nil {
		return nil, errors.ErrUnsupported
	}

	nodes, err := s.rg.GetUnreferencedNodes(ctx)
	resp := &block_gc_rpc.GetUnreferencedNodesResponse{}
	if err != nil {
		resp.Error = err.Error()
	} else {
		resp.Nodes = nodes
	}
	return resp, nil
}

// _ is a type assertion
var _ block_gc_rpc.SRPCRefGraphServer = ((*RefGraph)(nil))
