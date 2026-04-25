package s4wave_canvas

import (
	"context"
	"maps"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
)

// CanvasResource implements the CanvasResourceService SRPC interface.
type CanvasResource struct {
	ws     world.WorldState
	engine world.Engine
	objKey string
	state  *CanvasState
	bcast  broadcast.Broadcast
	mux    srpc.Mux
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
	r.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return SRPCRegisterCanvasResourceService(mux, r)
	})
	return r
}

// GetMux returns the srpc mux for this resource.
func (r *CanvasResource) GetMux() srpc.Mux {
	return r.mux
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
	var updated *CanvasState
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		updated = r.state.CloneVT()
	})

	if updated.Nodes == nil {
		updated.Nodes = make(map[string]*CanvasNode)
	}

	// Set/update nodes.
	maps.Copy(updated.Nodes, req.GetSetNodes())

	// Remove nodes.
	for _, id := range req.GetRemoveNodeIds() {
		delete(updated.Nodes, id)
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

	// Persist to the world if engine is available.
	if r.engine != nil {
		if err := r.persistState(ctx, updated); err != nil {
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
//
// Watches the underlying world object for revision changes so that
// updates from any source (other resource instances, world ops, etc.)
// are detected and streamed to the caller.
func (r *CanvasResource) WatchCanvasState(_ *WatchCanvasStateRequest, strm SRPCCanvasResourceService_WatchCanvasStateStream) error {
	ctx := strm.Context()

	// Watch the world object for changes from any source.
	objState, found, err := r.ws.GetObject(ctx, r.objKey)
	if err != nil {
		return err
	}
	if !found {
		return world.ErrObjectNotFound
	}

	var lastSent *CanvasState
	for {
		if err := ctx.Err(); err != nil {
			return err
		}

		// Get the current object revision.
		_, rev, err := objState.GetRootRef(ctx)
		if err != nil {
			return err
		}

		// Read current canvas state from the world object.
		var state *CanvasState
		_, _, err = world.AccessObjectState(ctx, objState, false, func(bcs *block.Cursor) error {
			var uerr error
			state, uerr = UnmarshalCanvasState(ctx, bcs)
			return uerr
		})
		if err != nil {
			return err
		}
		if state == nil {
			state = &CanvasState{}
		}

		// Update local state so GetCanvasState stays current.
		r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			r.state = state.CloneVT()
			broadcast()
		})

		// Send if changed from last sent.
		if lastSent == nil || !state.EqualVT(lastSent) {
			if serr := strm.Send(&WatchCanvasStateResponse{State: state.CloneVT()}); serr != nil {
				return serr
			}
			lastSent = state
		}

		// Wait for the next world object revision change.
		_, err = objState.WaitRev(ctx, rev+1, false)
		if err != nil {
			return err
		}
	}
}

// persistState writes the canvas state to the world via a write transaction.
func (r *CanvasResource) persistState(ctx context.Context, state *CanvasState) error {
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
		bcs.SetBlock(state, true)
		return nil
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

// _ is a type assertion
var _ SRPCCanvasResourceServiceServer = (*CanvasResource)(nil)
