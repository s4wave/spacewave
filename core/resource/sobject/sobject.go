package resource_sobject

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
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
	// appPluginIDs is the immutable application declaration supplied before publication.
	appPluginIDs []string
	// bindingRegistry carries binding changes from the Resource root.
	bindingRegistry *process_binding.BindingRegistry
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
	// Stream the mounted SharedObject health when it exposes a health watch.
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

	// Acquire the SharedObject state watch to derive loading and ready health.
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

// GetSharedObjectTrim reports how far this device's state can trim the
// history and which members hold it back.
func (r *SharedObjectResource) GetSharedObjectTrim(
	ctx context.Context,
	req *s4wave_sobject.GetSharedObjectTrimRequest,
) (*s4wave_sobject.SharedObjectTrim, error) {
	// Read the config, checkpoint and operations of the current state.
	snap, err := r.sharedObject.GetSharedObjectState(ctx)
	if err != nil {
		return nil, err
	}
	cfg, err := snap.GetConfig(ctx)
	if err != nil {
		return nil, err
	}
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		return nil, err
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		return nil, err
	}

	// Report the order, the stable point and this device's acknowledgment.
	viewer := r.sharedObject.GetPeerID().String()
	checkpointer := cfg.Checkpointer()
	roster := cfg.TrimRoster()
	trim := &s4wave_sobject.SharedObjectTrim{
		ViewerPeerId:        viewer,
		CheckpointHeight:    checkpoint.GetHeight(),
		Operations:          int64(set.Len()),
		Placed:              int64(len(set.Order())),
		Stable:              int64(len(set.StablePoint(roster))),
		SequencerPeerId:     cfg.GetSequencer().GetPeerId(),
		CheckpointerPeerId:  checkpointer,
		NeedsAcknowledgment: set.NeedsAcknowledgment(checkpointer, viewer, sobject.AcknowledgmentLag),
		UnplacedHeads:       set.UnplacedHeads(),
	}

	// Report each writer's latest operation and whether it built on the
	// checkpointer's.
	for _, peerID := range cfg.Writers() {
		nonce, _ := set.AuthorHead(peerID)
		trim.Writers = append(trim.Writers, &s4wave_sobject.SharedObjectTrimWriter{
			PeerId:              peerID,
			Dropped:             !slices.Contains(roster, peerID),
			LatestNonce:         nonce,
			BuiltOnCheckpointer: set.BuiltOnLatest(peerID, checkpointer),
		})
	}
	return trim, nil
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

// SetAppPluginIDs supplies application composition before the resource is published.
func (r *SharedObjectResource) SetAppPluginIDs(ids []string) {
	r.appPluginIDs = slices.Clone(ids)
}

// SetBindingRegistry supplies the Resource root's binding event source.
func (r *SharedObjectResource) SetBindingRegistry(registry *process_binding.BindingRegistry) {
	r.bindingRegistry = registry
}

// _ is a type assertion
var _ s4wave_sobject.SRPCSharedObjectResourceServiceServer = (*SharedObjectResource)(nil)
