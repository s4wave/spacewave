package resource_sobject

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	cdn_sharedobject "github.com/s4wave/spacewave/core/cdn/sharedobject"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	"github.com/sirupsen/logrus"
)

// SharedObjectResource wraps a core shared object for resource access.
type SharedObjectResource struct {
	le           *logrus.Entry
	b            bus.Bus
	mux          srpc.Invoker
	sharedObject sobject.SharedObject
	meta         *sobject.SharedObjectMeta
	ref          *sobject.SharedObjectRef
}

// NewSharedObjectResource creates a new SharedObjectResource.
func NewSharedObjectResource(
	le *logrus.Entry,
	b bus.Bus,
	so sobject.SharedObject,
	meta *sobject.SharedObjectMeta,
	ref *sobject.SharedObjectRef,
) *SharedObjectResource {
	soResource := &SharedObjectResource{le: le, b: b, sharedObject: so, meta: meta, ref: ref}
	soResource.mux = resource_server.NewResourceMux(func(mux srpc.Mux) error {
		return s4wave_sobject.SRPCRegisterSharedObjectResourceService(mux, soResource)
	})
	return soResource
}

// GetMux returns the rpc mux.
func (r *SharedObjectResource) GetMux() srpc.Invoker {
	return r.mux
}

// WatchSharedObjectHealth streams health for the mounted shared object.
func (r *SharedObjectResource) WatchSharedObjectHealth(
	req *s4wave_sobject.WatchSharedObjectHealthRequest,
	strm s4wave_sobject.SRPCSharedObjectResourceService_WatchSharedObjectHealthStream,
) error {
	ctx := strm.Context()
	if healthAccessor, ok := r.sharedObject.(sobject.SharedObjectHealthAccessor); ok {
		healthCtr, relHealthCtr, err := healthAccessor.AccessSharedObjectHealth(ctx, nil)
		if err != nil {
			return waitSharedObjectHealth(
				ctx,
				strm,
				sobject.BuildSharedObjectHealthFromError(
					sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
					err,
				),
			)
		}
		defer relHealthCtr()
		return watchSharedObjectHealthWatchable(ctx, strm, healthCtr)
	}

	stateCtr, relStateCtr, err := r.sharedObject.AccessSharedObjectState(ctx, nil)
	if err != nil {
		return waitSharedObjectHealth(
			ctx,
			strm,
			sobject.BuildSharedObjectHealthFromError(
				sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				err,
			),
		)
	}
	defer relStateCtr()

	return ccontainer.WatchChanges(
		ctx,
		nil,
		stateCtr,
		func(snap sobject.SharedObjectStateSnapshot) error {
			if snap == nil {
				return strm.Send(&s4wave_sobject.WatchSharedObjectHealthResponse{
					Health: sobject.NewSharedObjectLoadingHealth(
						sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
					),
				})
			}
			return strm.Send(&s4wave_sobject.WatchSharedObjectHealthResponse{
				Health: sobject.NewSharedObjectReadyHealth(
					sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				),
			})
		},
		nil,
	)
}

// MountSharedObjectBody mounts the body of a shared object.
func (r *SharedObjectResource) MountSharedObjectBody(ctx context.Context, req *s4wave_sobject.MountSharedObjectBodyRequest) (*s4wave_sobject.MountSharedObjectBodyResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// TODO: we switch over shared object body types here. add a directive to look up the factory?
	var resource srpc.Invoker
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

		spaceResource := resource_space.NewSpaceResource(r.le, r.b, mountedSpace.GetSharedObjectBody())
		resource, relResource = spaceResource.GetMux(), mountedSpaceRef.Release
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
		spaceResource := resource_space.NewSpaceResource(r.le, r.b, body)
		resource, relResource = spaceResource.GetMux(), we.Release
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

	id, err := resourceCtx.AddResource(resource, relResource)
	if err != nil {
		relResource()
		return nil, err
	}
	return &s4wave_sobject.MountSharedObjectBodyResponse{ResourceId: id}, nil
}

// watchSharedObjectHealthWatchable streams SharedObject health from a watchable.
func watchSharedObjectHealthWatchable(
	ctx context.Context,
	strm s4wave_sobject.SRPCSharedObjectResourceService_WatchSharedObjectHealthStream,
	healthCtr ccontainer.Watchable[*sobject.SharedObjectHealth],
) error {
	return ccontainer.WatchChanges(
		ctx,
		nil,
		healthCtr,
		func(health *sobject.SharedObjectHealth) error {
			if health == nil {
				health = sobject.NewSharedObjectLoadingHealth(
					sobject.SharedObjectHealthLayer_SHARED_OBJECT_HEALTH_LAYER_SHARED_OBJECT,
				)
			}
			return strm.Send(&s4wave_sobject.WatchSharedObjectHealthResponse{
				Health: health,
			})
		},
		nil,
	)
}

// waitSharedObjectHealth sends one health snapshot and waits for cancellation.
func waitSharedObjectHealth(
	ctx context.Context,
	strm s4wave_sobject.SRPCSharedObjectResourceService_WatchSharedObjectHealthStream,
	health *sobject.SharedObjectHealth,
) error {
	if err := strm.Send(&s4wave_sobject.WatchSharedObjectHealthResponse{
		Health: health,
	}); err != nil {
		return err
	}
	<-ctx.Done()
	return nil
}

// _ is a type assertion
var _ s4wave_sobject.SRPCSharedObjectResourceServiceServer = ((*SharedObjectResource)(nil))
