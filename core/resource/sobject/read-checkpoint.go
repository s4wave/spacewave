package resource_sobject

import (
	"context"

	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_world "github.com/s4wave/spacewave/core/resource/world"
	"github.com/s4wave/spacewave/core/sobject"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	s4wave_world "github.com/s4wave/spacewave/sdk/world"
)

// OpenReadCheckpoint opens retained World history without live body or write authority.
func (r *SharedObjectResource) OpenReadCheckpoint(
	ctx context.Context,
	_ *s4wave_sobject.OpenReadCheckpointRequest,
) (*s4wave_sobject.OpenReadCheckpointResponse, error) {
	accessor, ok := r.sharedObject.(sobject.SharedObjectReadCheckpointAccessor)
	if !ok {
		return &s4wave_sobject.OpenReadCheckpointResponse{}, nil
	}
	snapshot, err := accessor.GetSharedObjectReadCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot == nil {
		return &s4wave_sobject.OpenReadCheckpointResponse{}, nil
	}
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
	engine, release, err := sobject_world_engine.OpenReadCheckpoint(ctx, r.le, r.b, r.sharedObject, snapshot.Snapshot)
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
	return &s4wave_sobject.OpenReadCheckpointResponse{ResourceId: id, Config: snapshot.Config}, nil
}
