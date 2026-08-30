package s4wave_canvas

import (
	"context"
	"maps"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// CanvasResource implements the CanvasResourceService SRPC interface.
type CanvasResource struct {
	ws       world.WorldState
	engine   world.Engine
	objKey   string
	state    *CanvasState
	watch    *routine.RoutineContainer
	watchErr error
	closed   bool
	bcast    broadcast.Broadcast
	mux      srpc.Mux
}

type canvasWatchSnapshot struct {
	state *CanvasState
	err   error
}

// NewCanvasResource creates a new CanvasResource.
func NewCanvasResource(ws world.WorldState, engine world.Engine, objKey string, state *CanvasState) *CanvasResource {
	if state == nil {
		state = &CanvasState{}
	}
	r := &CanvasResource{
		ws:     ws,
		engine: engine,
		objKey: objKey,
		state:  state,
	}
	if ws != nil && objKey != "" {
		r.watch = routine.NewRoutineContainer()
		r.watch.SetRoutine(r.watchCanvasWorld)
		r.watch.SetContext(context.Background(), false)
	}
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterCanvasResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *CanvasResource) GetMux() srpc.Mux {
	return r.mux
}

// Close releases the canvas resource lifecycle.
func (r *CanvasResource) Close() {
	if r.watch != nil {
		r.watch.ClearContext()
	}
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.closed = true
		r.watchErr = context.Canceled
		broadcast()
	})
}

// GetCanvasState returns the current canvas state.
func (r *CanvasResource) GetCanvasState(_ context.Context, _ *GetCanvasStateRequest) (*GetCanvasStateResponse, error) {
	var state *CanvasState
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		state = r.state.CloneVT()
	})
	return &GetCanvasStateResponse{State: state}, nil
}

// UpdateCanvas applies a batch update to the canvas.
func (r *CanvasResource) UpdateCanvas(ctx context.Context, req *UpdateCanvasRequest) (*UpdateCanvasResponse, error) {
	// Apply mutations to a clone of the current state.
	var previous *CanvasState
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		previous = r.state.CloneVT()
	})
	updated := previous.CloneVT()

	if updated.Nodes == nil {
		updated.Nodes = make(map[string]*CanvasNode)
	}

	// Set/update nodes.
	maps.Copy(updated.Nodes, req.GetSetNodes())

	// Remove nodes.
	for _, id := range req.GetRemoveNodeIds() {
		delete(updated.Nodes, id)
		delete(updated.LayoutMetadata, id)
	}

	// Add edges.
	updated.Edges = append(updated.Edges, req.GetAddEdges()...)

	// Remove edges by ID.
	removeEdges := req.GetRemoveEdgeIds()
	if len(removeEdges) > 0 {
		removeSet := make(map[string]struct{}, len(removeEdges))
		for _, id := range removeEdges {
			removeSet[id] = struct{}{}
		}
		filtered := updated.Edges[:0]
		for _, e := range updated.Edges {
			if _, ok := removeSet[e.GetId()]; !ok {
				filtered = append(filtered, e)
			}
		}
		updated.Edges = filtered
	}

	// Add hidden graph links, deduplicating by structured identity.
	if addHidden := req.GetAddHiddenGraphLinks(); len(addHidden) > 0 {
		existing := make(map[hiddenGraphLinkKey]struct{}, len(updated.HiddenGraphLinks)+len(addHidden))
		for _, link := range updated.HiddenGraphLinks {
			existing[newHiddenGraphLinkKey(link)] = struct{}{}
		}
		for _, link := range addHidden {
			if link == nil {
				continue
			}
			key := newHiddenGraphLinkKey(link)
			if _, ok := existing[key]; ok {
				continue
			}
			updated.HiddenGraphLinks = append(updated.HiddenGraphLinks, link.CloneVT())
			existing[key] = struct{}{}
		}
	}

	// Remove hidden graph links by structured identity.
	if removeHidden := req.GetRemoveHiddenGraphLinks(); len(removeHidden) > 0 {
		removeSet := make(map[hiddenGraphLinkKey]struct{}, len(removeHidden))
		for _, link := range removeHidden {
			removeSet[newHiddenGraphLinkKey(link)] = struct{}{}
		}
		filtered := updated.HiddenGraphLinks[:0]
		for _, link := range updated.HiddenGraphLinks {
			if _, ok := removeSet[newHiddenGraphLinkKey(link)]; !ok {
				filtered = append(filtered, link)
			}
		}
		updated.HiddenGraphLinks = filtered
	}

	// Set layout metadata for updated nodes.
	if setLayout := req.GetSetLayoutMetadata(); len(setLayout) > 0 {
		if updated.LayoutMetadata == nil {
			updated.LayoutMetadata = make(map[string]*CanvasLayoutMetadata, len(setLayout))
		}
		for id, meta := range setLayout {
			if meta == nil {
				continue
			}
			updated.LayoutMetadata[id] = meta.CloneVT()
		}
	}

	// Remove layout metadata for deleted nodes.
	for _, id := range req.GetRemoveLayoutMetadataNodeIds() {
		delete(updated.LayoutMetadata, id)
	}

	// Persist to the world if engine is available.
	if r.engine != nil {
		if err := r.persistState(ctx, previous, updated); err != nil {
			return nil, errors.Wrap(err, "persist canvas state")
		}
	}

	// Update local state and broadcast.
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.state = updated
		broadcast()
	})

	return &UpdateCanvasResponse{State: updated.CloneVT()}, nil
}

// WatchCanvasState streams canvas state changes.
func (r *CanvasResource) WatchCanvasState(_ *WatchCanvasStateRequest, strm SRPCCanvasResourceService_WatchCanvasStateStream) error {
	return broadcast.WatchBroadcastWithEqual(
		strm.Context(),
		&r.bcast,
		r.snapshotCanvasWatchLocked,
		func(snap *canvasWatchSnapshot) error {
			if snap.err != nil {
				return snap.err
			}
			return strm.Send(&WatchCanvasStateResponse{State: snap.state.CloneVT()})
		},
		canvasWatchSnapshotsEqual,
	)
}

func (r *CanvasResource) watchCanvasWorld(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			r.setCanvasWatchError(err)
			return err
		}

		seqno, err := r.ws.GetSeqno(ctx)
		if err != nil {
			r.setCanvasWatchError(err)
			return err
		}

		objState, found, err := r.ws.GetObject(ctx, r.objKey)
		if err != nil {
			r.setCanvasWatchError(err)
			return err
		}
		if !found {
			r.setCanvasWatchError(world.ErrObjectNotFound)
			return world.ErrObjectNotFound
		}
		state, err := func() (*CanvasState, error) {
			// Release the object-state handle before the WaitSeqno block below
			// so the read scope does not span the wait.
			defer world.ReleaseObjectState(objState)
			return r.readCanvasWorldState(ctx, objState)
		}()
		if err != nil {
			r.setCanvasWatchError(err)
			return err
		}
		r.setCanvasWatchState(state)

		_, err = r.ws.WaitSeqno(ctx, seqno+1)
		if err != nil {
			r.setCanvasWatchError(err)
			return err
		}
	}
}

func (r *CanvasResource) readCanvasWorldState(ctx context.Context, objState world.ObjectState) (*CanvasState, error) {
	var state *CanvasState
	_, _, err := world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
		var uerr error
		state, uerr = UnmarshalCanvasState(ctx, bcs)
		return uerr
	})
	if err != nil {
		return nil, err
	}
	if state == nil {
		state = &CanvasState{}
	}
	return state, nil
}

func (r *CanvasResource) setCanvasWatchState(state *CanvasState) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.closed {
			return
		}
		r.state = state.CloneVT()
		r.watchErr = nil
		broadcast()
	})
}

func (r *CanvasResource) setCanvasWatchError(err error) {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.closed {
			return
		}
		r.watchErr = err
		broadcast()
	})
}

func (r *CanvasResource) snapshotCanvasWatchLocked() *canvasWatchSnapshot {
	if r.watchErr != nil {
		return &canvasWatchSnapshot{err: r.watchErr}
	}
	return &canvasWatchSnapshot{state: r.state.CloneVT()}
}

func canvasWatchSnapshotsEqual(a, b *canvasWatchSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.err != nil || b.err != nil {
		if a.err == nil || b.err == nil {
			return false
		}
		return a.err.Error() == b.err.Error()
	}
	if a.state == nil || b.state == nil {
		return a.state == b.state
	}
	return a.state.EqualVT(b.state)
}

// persistState writes the canvas state to the world via a write transaction.
func (r *CanvasResource) persistState(ctx context.Context, previous, next *CanvasState) error {
	wtx, err := r.engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	writeState, found, err := wtx.GetObject(ctx, r.objKey)
	if err != nil {
		wtx.Discard()
		return err
	}
	if !found {
		wtx.Discard()
		return world.ErrObjectNotFound
	}
	_, _, err = world.AccessObjectState(ctx, writeState, true, func(bcs *block.Cursor) error {
		return WriteCanvasState(ctx, bcs, previous, next)
	})
	if err != nil {
		wtx.Discard()
		return err
	}
	return wtx.Commit(ctx)
}

type hiddenGraphLinkKey struct {
	subject   string
	predicate string
	object    string
	label     string
}

func newHiddenGraphLinkKey(link *HiddenGraphLink) hiddenGraphLinkKey {
	return hiddenGraphLinkKey{
		subject:   link.GetSubject(),
		predicate: link.GetPredicate(),
		object:    link.GetObject(),
		label:     link.GetLabel(),
	}
}

var _ SRPCCanvasResourceServiceServer = (*CanvasResource)(nil)
