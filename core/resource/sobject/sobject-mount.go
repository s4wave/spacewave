//go:build !goscript

package resource_sobject

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
)

// MountSharedObjectBody mounts the body of a shared object.
func (r *SharedObjectResource) MountSharedObjectBody(ctx context.Context, req *s4wave_sobject.MountSharedObjectBodyRequest) (*s4wave_sobject.MountSharedObjectBodyResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: we switch over shared object body types here. add a directive to look up the factory?
	var resource srpc.Invoker
	var resourceValue any
	var relResource func()
	bodyType := r.meta.GetBodyType()
	switch bodyType {
	case space.SpaceBodyType:
		// TODO: pass release here?
		mountedSpace, mountedSpaceRef, err := space.ExMountSpaceSoBody(ctx, r.sharedObject.GetBus(), r.ref, false, nil)
		if err != nil {
			return nil, sobject.WrapSharedObjectHealthError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
				err,
			)
		}

		spaceResource := resource_space.NewSpaceResourceWithSessionPeerIDAndHostPluginID(
			r.le,
			r.b,
			mountedSpace.GetSharedObjectBody(),
			r.sessionPeerID,
			r.hostPluginID,
		)
		resource, relResource = spaceResource.GetMux(), mountedSpaceRef.Release
		resourceValue = spaceResource
	case cdn_sharedobject.CdnBodyType:
		cdnSO, ok := r.sharedObject.(*cdn_sharedobject.CdnSharedObject)
		if !ok {
			return nil, errors.Errorf("cdn body type on non-cdn shared object: %T", r.sharedObject)
		}
		we, err := cdn_sharedobject.NewWorldEngine(ctx, r.le, r.b, cdnSO, space_world_optypes.LookupWorldOp)
		if err != nil {
			return nil, sobject.WrapSharedObjectHealthError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
				errors.Wrap(err, "build cdn world engine"),
			)
		}
		body := cdn_sharedobject.NewCdnSpaceBody(cdnSO, we)
		spaceResource := resource_space.NewSpaceResourceWithSessionPeerIDAndHostPluginID(
			r.le,
			r.b,
			body,
			"",
			r.hostPluginID,
		)
		resource, relResource = spaceResource.GetMux(), we.Release
		resourceValue = spaceResource
	case "":
		return nil, sobject.WrapSharedObjectHealthError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
			sobject.ErrEmptyBodyType,
		)
	default:
		return nil, sobject.WrapSharedObjectHealthError(
			sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_BODY,
			errors.Errorf("unsupported shared object type: %v", bodyType),
		)
	}

	id, err := resourceCtx.AddResourceValue(resource, resourceValue, relResource)
	if err != nil {
		relResource()
		return nil, err
	}
	return &s4wave_sobject.MountSharedObjectBodyResponse{ResourceId: id}, nil
}
