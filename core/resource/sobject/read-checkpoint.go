package resource_sobject

import (
	"context"

	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/core/space"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// OpenReadCheckpoint opens retained World history without live body or write authority.
func (r *SharedObjectResource) OpenReadCheckpoint(
	ctx context.Context,
	_ *s4wave_sobject.OpenReadCheckpointRequest,
) (*s4wave_sobject.OpenReadCheckpointResponse, error) {
	// Read the held checkpoint, if any.
	accessor, ok := r.sharedObject.(sobject.SharedObjectReadCheckpointAccessor)
	if !ok {
		return &s4wave_sobject.OpenReadCheckpointResponse{}, nil
	}
	checkpoint, err := accessor.GetSharedObjectReadCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	if checkpoint == nil {
		return &s4wave_sobject.OpenReadCheckpointResponse{}, nil
	}

	// Resolve the client context and the session peer.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}
	sessionPeerID := peer.ID("")
	if r.sessionPeerID != "" {
		sessionPeerID, err = peer.IDB58Decode(r.sessionPeerID)
		if err != nil {
			return nil, err
		}
	}

	// Serve the checkpoint's World as an engine resource.
	engine, release, err := sobject_world_engine.OpenReadCheckpoint(ctx, r.le, r.b, r.sharedObject, space.SpaceEngineId(r.ref), checkpoint.Snapshot)
	if err != nil {
		return nil, err
	}
	resource := resource_world.NewEngineResource(r.le, r.b, engine, nil, &s4wave_world.EngineInfo{}, resource_world.WithSessionPeerID(sessionPeerID))
	releaseResource := func() {
		resource.Close()
		release()
	}
	id, err := resourceCtx.AddResource(resource.GetMux(), releaseResource)
	if err != nil {
		releaseResource()
		return nil, err
	}
	return &s4wave_sobject.OpenReadCheckpointResponse{ResourceId: id, Config: checkpoint.Config}, nil
}
