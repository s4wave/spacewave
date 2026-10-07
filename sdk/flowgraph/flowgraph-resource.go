package s4wave_flowgraph

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// FlowgraphResource exposes an authored graph through its granted World capability.
type FlowgraphResource struct {
	// ws preserves the granting mount's snapshot and write authority.
	ws world.WorldState
	// engine opens transactions when the granting mount permits edits.
	engine world.Engine
	// key identifies this independently addressed graph.
	key string
	// mux serves the graph's Resource operations.
	mux srpc.Mux
}

// NewFlowgraphResource constructs a graph Resource without retaining object handles.
func NewFlowgraphResource(ws world.WorldState, engine world.Engine, key string) *FlowgraphResource {
	r := &FlowgraphResource{ws: ws, engine: engine, key: key}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterFlowgraphResourceService(mux, r)
	})
	return r
}

// GetMux returns the graph's Resource invoker.
func (r *FlowgraphResource) GetMux() srpc.Mux {
	return r.mux
}

// GetFlowgraph reads the granted snapshot or a fresh committed graph.
func (r *FlowgraphResource) GetFlowgraph(ctx context.Context, _ *GetFlowgraphRequest) (*GetFlowgraphResponse, error) {
	// Preserve read-only snapshots without upgrading through the engine.
	ws := r.ws
	if !ws.GetReadOnly() && r.engine != nil {
		read, err := r.engine.NewTransaction(ctx, false)
		if err != nil {
			return nil, err
		}
		defer read.Discard()
		ws = read
	}

	// Read the body and placement edges through one granted state.
	snapshot, err := ReadFlowgraph(ctx, ws, r.key)
	if err != nil {
		return nil, err
	}
	return &GetFlowgraphResponse{Snapshot: snapshot}, nil
}

// UpdateFlowgraph commits one authored change and returns that change's snapshot.
func (r *FlowgraphResource) UpdateFlowgraph(ctx context.Context, request *UpdateFlowgraphRequest) (*UpdateFlowgraphResponse, error) {
	// Require the granting mount's write capability before opening a transaction.
	if r.ws.GetReadOnly() || r.engine == nil {
		return nil, tx.ErrNotWrite
	}
	write, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return nil, err
	}
	defer write.Discard()

	// Apply the edit to the current graph and wait for durable commit.
	snapshot, err := UpdateFlowgraph(ctx, write, r.key, request)
	if err != nil {
		return nil, err
	}
	if err := write.Commit(ctx); err != nil {
		return nil, err
	}
	return &UpdateFlowgraphResponse{Snapshot: snapshot}, nil
}

// WatchFlowgraph streams committed graph changes until the stream is canceled.
func (r *FlowgraphResource) WatchFlowgraph(_ *WatchFlowgraphRequest, stream SRPCFlowgraphResourceService_WatchFlowgraphStream) error {
	// Retain a live object handle for key-scoped revision notifications.
	ctx := stream.Context()
	obj, found, err := r.ws.GetObject(ctx, r.key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	// Read and emit one consistent snapshot before awaiting its successor.
	for {
		response, err := r.GetFlowgraph(ctx, &GetFlowgraphRequest{})
		if err != nil {
			return err
		}
		if err := stream.Send(&WatchFlowgraphResponse{Snapshot: response.GetSnapshot()}); err != nil {
			return err
		}
		if _, err := obj.WaitRev(ctx, response.GetSnapshot().GetRevision()+1, false); err != nil {
			return err
		}
	}
}

// _ is a type assertion.
var _ SRPCFlowgraphResourceServiceServer = (*FlowgraphResource)(nil)
