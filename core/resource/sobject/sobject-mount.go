//go:build !goscript

package resource_sobject

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

// MountSharedObjectBody mounts the body of a shared object.
func (r *SharedObjectResource) MountSharedObjectBody(ctx context.Context, req *s4wave_sobject.MountSharedObjectBodyRequest) (*s4wave_sobject.MountSharedObjectBodyResponse, error) {
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
			errors.Errorf("unsupported shared object type: %v", bodyType),
		)), nil
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
			errors.Errorf("unsupported shared object type: %v", bodyType),
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
