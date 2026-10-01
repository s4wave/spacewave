package resource_sobject

import (
	"context"
	"fmt"
	"slices"

	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

// MountSharedObjectBody mounts the body of a shared object.
func (r *SharedObjectResource) MountSharedObjectBody(ctx context.Context, req *s4wave_sobject.MountSharedObjectBodyRequest) (response *s4wave_sobject.MountSharedObjectBodyResponse, mountErr error) {
	// Preserve health from all body providers, including failures before mounting.
	defer func() {
		if mountErr != nil && ctx.Err() == nil {
			response = mountSharedObjectBodyHealthResponse(mountErr)
			mountErr = nil
		}
	}()

	// Find the caller's resource client.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Require a body type and a SharedObject bus to mount it on.
	bodyType := r.meta.GetBodyType()
	if bodyType == "" {
		return mountSharedObjectBodyHealthResponse(sobject.WrapSharedObjectHealthError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
			sobject.ErrEmptyBodyType,
		)), nil
	}
	if r.sharedObject == nil || r.sharedObject.GetBus() == nil {
		return mountSharedObjectBodyHealthResponse(sobject.WrapSharedObjectHealthError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
			fmt.Errorf("%w: %s", sobject.ErrUnsupportedBodyType, bodyType),
		)), nil
	}

	// Retained local data does not grant a departed participant a readable body.
	if host, ok := r.sharedObject.(sobject.InviteHost); ok {
		state, err := host.GetSOHost().GetHostState(ctx)
		if err != nil {
			return nil, err
		}
		config := state.GetConfig()
		readable := slices.ContainsFunc(config.GetParticipants(), func(participant *sobject.SOParticipantConfig) bool {
			return participant.GetPeerId() == r.sharedObject.GetPeerID().String() && sobject.CanReadState(participant.GetRole())
		})
		if healthAccessor, ok := r.sharedObject.(sobject.SharedObjectHealthAccessor); ok {
			health, release, err := healthAccessor.AccessSharedObjectHealth(ctx, nil)
			if err != nil {
				return nil, err
			}
			readable = readable && !sobject.AuthoritativeSyncDenied(config, health.GetValue())
			release()
		}
		if !readable {
			return mountSharedObjectBodyHealthResponse(sobject.WrapSharedObjectHealthError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
				sobject.ErrNotParticipant,
			)), nil
		}
	}

	// Mount the Space body.
	mountedSpace, mountedSpaceRef, err := sobject.ExMountSharedObjectBodyWithSource[space.SpaceSharedObjectBody](
		ctx,
		r.sharedObject.GetBus(),
		r.ref,
		bodyType,
		r.sharedObject,
		true,
		nil,
	)
	if err != nil {
		return mountSharedObjectBodyHealthResponse(sobject.WrapSharedObjectHealthError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
			err,
		)), nil
	}
	if mountedSpace == nil {
		return mountSharedObjectBodyHealthResponse(sobject.WrapSharedObjectHealthError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
			fmt.Errorf("%w: %s", sobject.ErrUnsupportedBodyType, bodyType),
		)), nil
	}

	// Serve the Space resource until the client releases it.
	body := mountedSpace.GetSharedObjectBody()
	spaceResource := resource_space.NewSpaceResourceWithSessionPeerIDAndHostPluginID(
		r.le,
		r.b,
		body,
		resource_space.MountedBodySessionPeerID(body, r.sessionPeerID),
		r.hostPluginID,
	)
	spaceResource.SetAppPluginIDs(r.appPluginIDs)
	spaceResource.SetBindingRegistry(r.bindingRegistry)
	id, err := resourceCtx.AddResourceValue(spaceResource.GetMux(), spaceResource, mountedSpaceRef.Release)
	if err != nil {
		mountedSpaceRef.Release()
		return nil, err
	}
	return &s4wave_sobject.MountSharedObjectBodyResponse{
		Result: &s4wave_sobject.MountSharedObjectBodyResponse_ResourceId{
			ResourceId: id,
		},
	}, nil
}

func mountSharedObjectBodyHealthResponse(err error) *s4wave_sobject.MountSharedObjectBodyResponse {
	health, ok := sobject.GetSharedObjectHealthFromError(err)
	if !ok {
		health = sobject.BuildSharedObjectHealthFromError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
			err,
		)
	}
	return &s4wave_sobject.MountSharedObjectBodyResponse{
		Result: &s4wave_sobject.MountSharedObjectBodyResponse_Health{
			Health: health,
		},
	}
}
