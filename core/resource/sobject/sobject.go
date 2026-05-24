package resource_sobject

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_sobject "github.com/s4wave/spacewave/sdk/sobject"
	"github.com/sirupsen/logrus"
)

// SharedObjectResource wraps a core shared object for resource access.
type SharedObjectResource struct {
	le            *logrus.Entry
	b             bus.Bus
	mux           srpc.Invoker
	sharedObject  sobject.SharedObject
	meta          *sobject.SharedObjectMeta
	ref           *sobject.SharedObjectRef
	sessionPeerID string
	hostPluginID  string
}

// NewSharedObjectResource creates a new SharedObjectResource.
func NewSharedObjectResource(
	le *logrus.Entry,
	b bus.Bus,
	so sobject.SharedObject,
	meta *sobject.SharedObjectMeta,
	ref *sobject.SharedObjectRef,
	sessionPeerID string,
) *SharedObjectResource {
	return NewSharedObjectResourceWithHostPluginID(le, b, so, meta, ref, sessionPeerID, "")
}

// NewSharedObjectResourceWithHostPluginID creates a new SharedObjectResource
// with the plugin id that owns the resource root.
func NewSharedObjectResourceWithHostPluginID(
	le *logrus.Entry,
	b bus.Bus,
	so sobject.SharedObject,
	meta *sobject.SharedObjectMeta,
	ref *sobject.SharedObjectRef,
	sessionPeerID string,
	hostPluginID string,
) *SharedObjectResource {
	soResource := &SharedObjectResource{
		le:            le,
		b:             b,
		sharedObject:  so,
		meta:          meta,
		ref:           ref,
		sessionPeerID: sessionPeerID,
		hostPluginID:  hostPluginID,
	}
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
