package s4wave_flowgraph

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
)

// FlowgraphResource exposes an authored graph through its granted World capability.
type FlowgraphResource struct {
	// ws preserves the granting mount's snapshot and write authority.
	ws world.WorldState
	// engine opens transactions when the granting mount permits edits.
	engine world.Engine
	// bus resolves the object types of bound objects. It may be nil, which
	// leaves every bound object's type unregistered.
	bus bus.Bus
	// engineID scopes the object type lookup to the granting World engine.
	engineID string
	// key identifies this independently addressed graph.
	key string
	// sessionPeerID identifies the granting Session that opens node streams.
	sessionPeerID peer.ID
	// mux serves the graph's Resource operations.
	mux srpc.Mux
}

// NewFlowgraphResource constructs a graph Resource without retaining object handles.
// b and engineID resolve the object types of the nodes' bound objects.
// sessionPeerID selects the granting Session for port calls; empty disables them.
func NewFlowgraphResource(ws world.WorldState, engine world.Engine, b bus.Bus, engineID, key string, sessionPeerID peer.ID) *FlowgraphResource {
	r := &FlowgraphResource{ws: ws, engine: engine, bus: b, engineID: engineID, key: key, sessionPeerID: sessionPeerID}
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
	// Read the body and placement edges through one granted state.
	ws, release, err := r.readState(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	snapshot, err := r.readSnapshot(ctx, ws)
	if err != nil {
		return nil, err
	}
	return &GetFlowgraphResponse{Snapshot: snapshot}, nil
}

// readSnapshot reads the graph and derives the binding of each bound node from
// the same World state.
func (r *FlowgraphResource) readSnapshot(ctx context.Context, ws world.WorldState) (*FlowgraphSnapshot, error) {
	snapshot, err := ReadFlowgraph(ctx, ws, r.key)
	if err != nil {
		return nil, err
	}
	snapshot.Bindings, err = ReadBindings(ctx, ws, r.bus, r.engineID, snapshot.GetState())
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

// readState returns the granted read-only snapshot or a fresh committed read.
// The caller calls release when the read completes.
func (r *FlowgraphResource) readState(ctx context.Context) (world.WorldState, func(), error) {
	if r.ws.GetReadOnly() || r.engine == nil {
		return r.ws, func() {}, nil
	}
	read, err := r.engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, nil, err
	}
	return read, read.Discard, nil
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
	snapshot, err := UpdateBoundFlowgraph(ctx, write, r.bus, r.engineID, r.key, request)
	if err != nil {
		return nil, err
	}
	if err := write.Commit(ctx); err != nil {
		return nil, err
	}
	return &UpdateFlowgraphResponse{Snapshot: snapshot}, nil
}

// ListFlowgraphChanges reads the graph's revisions from the World changelog.
func (r *FlowgraphResource) ListFlowgraphChanges(ctx context.Context, request *ListFlowgraphChangesRequest) (*ListFlowgraphChangesResponse, error) {
	ws, release, err := r.readState(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	return ReadFlowgraphChanges(ctx, ws, r.key, request.GetLimit())
}

// WatchFlowgraph streams committed graph changes until the stream is canceled.
// It also sends a snapshot when a bound object appears, changes type or is
// deleted, or when a plugin registers or releases the type of a bound object,
// since each of those changes the node's derived binding.
func (r *FlowgraphResource) WatchFlowgraph(_ *WatchFlowgraphRequest, stream SRPCFlowgraphResourceService_WatchFlowgraphStream) error {
	// Follow the registry for the bound objects' types while the stream runs.
	ctx := stream.Context()
	watches := newTypeWatches(r.bus, r.engineID)
	defer watches.close()
	var sent *FlowgraphSnapshot
	for {
		// Read the snapshot from one World state and record what it read, so a
		// change after the read to any of it ends the wait below.
		snapshot, reads, seqno, err := r.readWatched(ctx)
		if err != nil {
			return err
		}

		// Send the snapshot only when it differs from the last one sent, since
		// a bound object's own edits do not change the graph's view of it.
		if !snapshot.EqualVT(sent) {
			if err := stream.Send(&WatchFlowgraphResponse{Snapshot: snapshot}); err != nil {
				return err
			}
			sent = snapshot
		}

		// Watch the registry for the types bound objects have, and read again
		// after a new watch since it saw no change before it began.
		added, err := watches.sync(ctx, snapshot)
		if err != nil {
			return err
		}
		if added {
			continue
		}
		if err := waitGraphChange(ctx, r.ws, watches.wake, reads, seqno); err != nil {
			return err
		}
	}
}

// waitGraphChange returns when a World change after seqno touches reads or a
// registry change arrives on wake, and returns the context's error when ctx
// ends.
func waitGraphChange(ctx context.Context, ws world.WorldState, wake <-chan struct{}, reads *world.ReadSet, seqno uint64) error {
	// End the World wait when the registry wakes the stream.
	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-wake:
			cancel()
		case <-waitCtx.Done():
		}
	}()

	// Wait for a change to the reads, then stop the registry waker.
	_, err := reads.WaitChange(waitCtx, ws, seqno)
	cancel()
	<-done

	// A registry wake ends the wait without an error.
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil && waitCtx.Err() == nil {
		return err
	}
	return nil
}

// readWatched reads a snapshot and records its reads: the graph, its placement
// edges, and each bound object with its type edge. It returns the seqno the
// reads observed.
func (r *FlowgraphResource) readWatched(ctx context.Context) (*FlowgraphSnapshot, *world.ReadSet, uint64, error) {
	// Open the read and note the seqno it observes.
	ws, release, err := r.readState(ctx)
	if err != nil {
		return nil, nil, 0, err
	}
	defer release()
	seqno, err := ws.GetSeqno(ctx)
	if err != nil {
		return nil, nil, 0, err
	}

	// Read the snapshot through the recorded state.
	reads := world.NewReadSet()
	snapshot, err := r.readSnapshot(ctx, reads.Track(ws))
	if err != nil {
		return nil, nil, 0, err
	}
	return snapshot, reads, seqno, nil
}

// _ is a type assertion.
var _ SRPCFlowgraphResourceServiceServer = (*FlowgraphResource)(nil)
