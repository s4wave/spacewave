package resource_sobject

import (
	"context"
	"fmt"
	"slices"

	"github.com/aperturerobotics/starpc/srpc"
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

	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	var resource srpc.Invoker
	var resourceValue any
	var relResource func()
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

	body := mountedSpace.GetSharedObjectBody()
	spaceResource := resource_space.NewSpaceResourceWithSessionPeerIDAndHostPluginID(
		r.le,
		r.b,
		body,
		mountedBodySessionPeerID(body, r.sessionPeerID),
		r.hostPluginID,
	)
	spaceResource.SetAppPluginIDs(r.appPluginIDs)
	resource, relResource = spaceResource.GetMux(), mountedSpaceRef.Release
	resourceValue = spaceResource

	id, err := resourceCtx.AddResourceValue(resource, resourceValue, relResource)
	if err != nil {
		relResource()
		return nil, err
	}
	return &s4wave_sobject.MountSharedObjectBodyResponse{
		Result: &s4wave_sobject.MountSharedObjectBodyResponse_ResourceId{
			ResourceId: id,
		},
	}, nil
}

func mountedBodySessionPeerID(body space.SpaceSharedObjectBody, sessionPeerID string) string {
	if body == nil {
		return sessionPeerID
	}
	if so := body.GetSharedObject(); so != nil && so.GetPeerID() == "" {
		return ""
	}
	return sessionPeerID
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
