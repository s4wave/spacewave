package resource_sobject

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	resource_space "github.com/s4wave/spacewave/core/resource/space"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/core/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
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
	// accountPlugins follows the account plugin list for this resource's life.
	// It is nil without an account.
	accountPlugins resource_space.AccountPluginSource
	// bindingRegistry is the Resource root's binding event source for this resource's life.
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
	return NewSharedObjectResourceWithHostPluginID(le, b, so, meta, ref, sessionPeerID, "", nil, nil)
}

// NewSharedObjectResourceWithHostPluginID creates a new SharedObjectResource
// with the plugin ID that owns the resource root and fixed plugin dependencies.
// accountPlugins is nil when the resource has no account.
// bindingRegistry is nil when the resource has no binding event source.
func NewSharedObjectResourceWithHostPluginID(
	le *logrus.Entry,
	b bus.Bus,
	so sobject.SharedObject,
	meta *sobject.SharedObjectMeta,
	ref *sobject.SharedObjectRef,
	sessionPeerID string,
	hostPluginID string,
	accountPlugins resource_space.AccountPluginSource,
	bindingRegistry *process_binding.BindingRegistry,
) *SharedObjectResource {
	// Retain the mounted resource and its dependencies before exposing RPC methods.
	soResource := &SharedObjectResource{
		le:              le,
		b:               b,
		sharedObject:    so,
		meta:            meta,
		ref:             ref,
		sessionPeerID:   sessionPeerID,
		hostPluginID:    hostPluginID,
		accountPlugins:  accountPlugins,
		bindingRegistry: bindingRegistry,
	}

	// Register RPC methods on the fully configured resource.
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

	// Index the trim roster once for the writer membership checks.
	rosterMembers := make(map[string]bool, len(roster))
	for _, peerID := range roster {
		rosterMembers[peerID] = true
	}

	// Report each writer's latest operation and whether it built on the
	// checkpointer's.
	for _, peerID := range cfg.Writers() {
		nonce, _ := set.AuthorHead(peerID)
		trim.Writers = append(trim.Writers, &s4wave_sobject.SharedObjectTrimWriter{
			PeerId:              peerID,
			Dropped:             !rosterMembers[peerID],
			LatestNonce:         nonce,
			BuiltOnCheckpointer: set.BuiltOnLatest(peerID, checkpointer),
		})
	}
	return trim, nil
}

// InspectSharedObjectOrder walks the mounted Space's agreed order from its
// checkpoint to the head and sends what this build reads of each operation the
// request selects, then the summary. It writes nothing.
func (r *SharedObjectResource) InspectSharedObjectOrder(
	req *s4wave_sobject.InspectSharedObjectOrderRequest,
	strm s4wave_sobject.SRPCSharedObjectResourceService_InspectSharedObjectOrderStream,
) error {
	// Only a Space's body is a World order.
	if bodyType := r.meta.GetBodyType(); bodyType != space.SpaceBodyType {
		return errors.Wrap(sobject.ErrUnsupportedBodyType, bodyType)
	}

	// Send each selected operation as the walk reads it, then the summary.
	summary, err := space_world.InspectOrder(strm.Context(), r.sharedObject, func(op *space_world.OrderOperation) error {
		if !req.GetAll() && !op.Undecodable() && !op.SavedUnknownType() {
			return nil
		}
		return strm.Send(&s4wave_sobject.InspectSharedObjectOrderResponse{
			Body: &s4wave_sobject.InspectSharedObjectOrderResponse_Operation{Operation: op},
		})
	})
	if err != nil {
		return err
	}
	return strm.Send(&s4wave_sobject.InspectSharedObjectOrderResponse{
		Body: &s4wave_sobject.InspectSharedObjectOrderResponse_Summary{Summary: summary},
	})
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

// _ is a type assertion
var _ s4wave_sobject.SRPCSharedObjectResourceServiceServer = (*SharedObjectResource)(nil)
