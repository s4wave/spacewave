package resource_space

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_approval "github.com/s4wave/spacewave/core/plugin/approval"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// SpaceContentsResource provides streaming plugin status for a mounted space.
type SpaceContentsResource struct {
	le       *logrus.Entry
	b        bus.Bus
	mux      srpc.Invoker
	engine   world.Engine
	spaceID  string
	engineID string
	volumeID string
	storeID  string
	// ctrlRef holds the plugin/space controller reference.
	// Released when the resource is cleaned up.
	ctrlRef directive.Reference
	// ctrl wakes the running plugin/space controller after approval changes.
	ctrl *plugin_space.Controller
	// bcast is broadcast when approval state changes so WatchState re-sends.
	// Also guards the cached plugin description summary below.
	bcast broadcast.Broadcast
	// descriptionPluginIDs is the plugin ID set for the cached descriptions.
	descriptionPluginIDs []string
	// descriptions caches plugin descriptions for the current plugin set.
	descriptions map[string]string
	// buildDescriptions overrides description lookup in tests.
	buildDescriptions func(context.Context, world.WorldState, []string) (map[string]string, error)
}

// NewSpaceContentsResource creates a new SpaceContentsResource.
func NewSpaceContentsResource(le *logrus.Entry, b bus.Bus, engine world.Engine, spaceID, engineID string) *SpaceContentsResource {
	r := &SpaceContentsResource{
		le:       le,
		b:        b,
		engine:   engine,
		spaceID:  spaceID,
		engineID: engineID,
	}
	mux := srpc.NewMux()
	_ = s4wave_space.SRPCRegisterSpaceContentsResourceService(mux, r)
	r.mux = mux
	return r
}

// Release releases the controller reference.
func (r *SpaceContentsResource) Release() {
	if r.ctrlRef != nil {
		r.ctrlRef.Release()
		r.ctrlRef = nil
	}
}

// notifyChanged signals WatchState to re-read and re-send.
func (r *SpaceContentsResource) notifyChanged() {
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		broadcast()
	})
}

func (r *SpaceContentsResource) getStoreLocation() (string, string) {
	volumeID := r.volumeID
	if volumeID == "" {
		volumeID = bldr_plugin.PluginVolumeID
	}
	storeID := r.storeID
	if storeID == "" {
		storeID = plugin_approval.DefaultObjectStoreID
	}
	return volumeID, storeID
}

func (r *SpaceContentsResource) notifyController() {
	if r.ctrl != nil {
		r.ctrl.NotifyChanged()
	}
}

// GetMux returns the rpc mux.
func (r *SpaceContentsResource) GetMux() srpc.Invoker {
	return r.mux
}

// WatchState streams the current plugin approval states for the space.
func (r *SpaceContentsResource) WatchState(
	req *s4wave_space.WatchSpaceContentsStateRequest,
	strm s4wave_space.SRPCSpaceContentsResourceService_WatchStateStream,
) error {
	ctx := strm.Context()

	var prevSeqno uint64
	for {
		// Read SpaceSettings and manifest descriptions from the world.
		var pluginIDs []string
		var descriptions map[string]string
		if err := func() error {
			wtx, err := r.engine.NewTransaction(ctx, false)
			if err != nil {
				return err
			}
			defer wtx.Discard()

			prevSeqno, err = wtx.GetSeqno(ctx)
			if err != nil {
				return err
			}

			settings, _, err := space_world.LookupSpaceSettings(ctx, wtx)
			if err != nil {
				return err
			}
			if settings != nil {
				pluginIDs = settings.GetPluginIds()
			}

			descriptions, err = r.getPluginDescriptions(ctx, wtx, pluginIDs)
			if err != nil {
				r.le.WithError(err).Warn("failed to resolve plugin descriptions")
				descriptions = nil
			}

			return nil
		}(); err != nil {
			return err
		}

		// Build plugin statuses.
		plugins := make([]*s4wave_space.SpacePluginStatus, 0, len(pluginIDs))
		for _, pid := range pluginIDs {
			state, err := plugin_approval.GetApprovalState(ctx, r.b, "", "", r.spaceID, pid)
			if err != nil {
				r.le.WithError(err).Warnf("failed to get approval state for plugin %s", pid)
				state = plugin_approval.PluginApprovalState_PluginApprovalState_UNSPECIFIED
			}
			plugins = append(plugins, &s4wave_space.SpacePluginStatus{
				PluginId:      pid,
				ApprovalState: state,
				Description:   descriptions[pid],
			})
		}
		processBindings, err := r.listProcessBindingInfos(ctx)
		if err != nil {
			return err
		}

		if err := strm.Send(&s4wave_space.SpaceContentsState{
			Ready:           true,
			Plugins:         plugins,
			ProcessBindings: processBindings,
		}); err != nil {
			return err
		}

		// Wait for world seqno change or approval state change.
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})
		waitCtx, waitCancel := context.WithCancel(ctx)
		go func() {
			select {
			case <-ch:
				waitCancel()
			case <-waitCtx.Done():
			}
		}()
		_, err = r.engine.WaitSeqno(waitCtx, prevSeqno+1)
		waitCancel()
		if err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
	}
}

// getPluginDescriptions returns cached plugin descriptions for the current plugin set.
func (r *SpaceContentsResource) getPluginDescriptions(
	ctx context.Context,
	ws world.WorldState,
	pluginIDs []string,
) (map[string]string, error) {
	pluginIDs = slices.Clone(pluginIDs)

	var cached map[string]string
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if slices.Equal(r.descriptionPluginIDs, pluginIDs) {
			cached = maps.Clone(r.descriptions)
		}
	})
	if cached != nil {
		return cached, nil
	}

	buildDescriptions := r.buildDescriptions
	if buildDescriptions == nil {
		buildDescriptions = r.collectPluginDescriptions
	}
	descriptions, err := buildDescriptions(ctx, ws, pluginIDs)
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		r.descriptionPluginIDs = slices.Clone(pluginIDs)
		r.descriptions = maps.Clone(descriptions)
	})
	return maps.Clone(descriptions), nil
}

// collectPluginDescriptions builds a description summary for the current plugin set.
func (r *SpaceContentsResource) collectPluginDescriptions(
	ctx context.Context,
	ws world.WorldState,
	pluginIDs []string,
) (map[string]string, error) {
	descriptions := make(map[string]string, len(pluginIDs))
	if len(pluginIDs) == 0 {
		return descriptions, nil
	}

	needed := make(map[string]struct{}, len(pluginIDs))
	for _, pid := range pluginIDs {
		if pid != "" {
			needed[pid] = struct{}{}
		}
	}
	if len(needed) == 0 {
		return descriptions, nil
	}

	manifestKeys, err := world_types.ListObjectsWithType(ctx, ws, bldr_manifest_world.ManifestTypeID)
	if err != nil {
		return nil, err
	}
	for _, key := range manifestKeys {
		m, _, err := bldr_manifest_world.LookupManifest(ctx, ws, key)
		if err != nil {
			continue
		}
		meta := m.GetMeta()
		mid := meta.GetManifestId()
		if _, ok := needed[mid]; !ok {
			continue
		}
		if _, ok := descriptions[mid]; ok {
			continue
		}
		if desc := meta.GetDescription(); desc != "" {
			descriptions[mid] = desc
		}
		if len(descriptions) == len(needed) {
			break
		}
	}

	return descriptions, nil
}

// SetPluginApproval sets the approval state for a plugin in this space.
func (r *SpaceContentsResource) SetPluginApproval(
	ctx context.Context,
	req *s4wave_space.SetPluginApprovalRequest,
) (*s4wave_space.SetPluginApprovalResponse, error) {
	pid := req.GetPluginId()
	if pid == "" {
		return nil, errors.New("plugin_id is required")
	}

	state := plugin_approval.PluginApprovalState_PluginApprovalState_DENIED
	if req.GetApproved() {
		state = plugin_approval.PluginApprovalState_PluginApprovalState_APPROVED
	}

	volumeID, storeID := r.getStoreLocation()
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		r.b,
		true,
		storeID,
		volumeID,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	approval := &plugin_approval.PluginApproval{State: state}
	if err := plugin_approval.SetPluginApproval(ctx, handle.GetObjectStore(), r.spaceID, pid, approval); err != nil {
		return nil, err
	}

	r.notifyChanged()
	r.notifyController()
	return &s4wave_space.SetPluginApprovalResponse{}, nil
}

// SetProcessBinding sets the approval state for a process binding.
func (r *SpaceContentsResource) SetProcessBinding(
	ctx context.Context,
	req *s4wave_space.SetProcessBindingRequest,
) (*s4wave_space.SetProcessBindingResponse, error) {
	objKey := req.GetObjectKey()
	if objKey == "" {
		return nil, errors.New("object_key is required")
	}
	typeID := req.GetTypeId()
	if typeID == "" {
		return nil, errors.New("type_id is required")
	}

	volumeID, storeID := r.getStoreLocation()
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		r.b,
		true,
		storeID,
		volumeID,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	state := s4wave_process.ProcessBindingState_ProcessBindingState_UNAPPROVED
	if req.GetApproved() {
		state = s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED
	}

	binding := &s4wave_process.ProcessBinding{
		State:     state,
		ObjectKey: objKey,
		TypeId:    typeID,
		DecidedAt: timestamppb.Now(),
	}
	if err := process_binding.SetProcessBinding(ctx, handle.GetObjectStore(), r.spaceID, objKey, binding); err != nil {
		return nil, err
	}

	r.notifyChanged()
	r.notifyController()
	return &s4wave_space.SetProcessBindingResponse{}, nil
}

func (r *SpaceContentsResource) listProcessBindingInfos(
	ctx context.Context,
) ([]*s4wave_space.ProcessBindingInfo, error) {
	volumeID, storeID := r.getStoreLocation()
	handle, _, ref, err := volume.ExBuildObjectStoreAPI(
		ctx,
		r.b,
		true,
		storeID,
		volumeID,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer ref.Release()

	bindings, err := process_binding.ListProcessBindings(ctx, handle.GetObjectStore(), r.spaceID)
	if err != nil {
		return nil, err
	}

	infos := make([]*s4wave_space.ProcessBindingInfo, 0, len(bindings))
	for _, b := range bindings {
		infos = append(infos, &s4wave_space.ProcessBindingInfo{
			ObjectKey: b.GetObjectKey(),
			TypeId:    b.GetTypeId(),
			Approved:  b.GetState() == s4wave_process.ProcessBindingState_ProcessBindingState_APPROVED,
			DecidedAt: b.GetDecidedAt(),
		})
	}

	return infos, nil
}

// _ is a type assertion
var _ s4wave_space.SRPCSpaceContentsResourceServiceServer = ((*SpaceContentsResource)(nil))
