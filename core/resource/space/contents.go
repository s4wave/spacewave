package resource_space

import (
	"cmp"
	"context"
	"errors"
	"maps"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

type spaceContentsControllerStarter func(
	ctx context.Context,
	b bus.Bus,
	conf *plugin_space.Config,
) (*plugin_space.Controller, directive.Reference, error)

// SpaceContentsResource provides streaming plugin status for a mounted space.
type SpaceContentsResource struct {
	le        *logrus.Entry
	b         bus.Bus
	mux       srpc.Invoker
	engine    world.Engine
	spaceID   string
	engineID  string
	volumeID  string
	storeID   string
	ctx       context.Context
	ctxCancel context.CancelFunc
	start     *routine.RoutineContainer
	startSeq  uint64
	released  bool
	// ctrlRef holds the plugin/space controller reference.
	// Released when the resource is cleaned up.
	ctrlRef directive.Reference
	// ctrl wakes the running plugin/space controller after content changes.
	ctrl *plugin_space.Controller
	// bcast is broadcast when content state changes so WatchState re-sends. It
	// also guards the cached plugin descriptions and manifest catalog below.
	bcast broadcast.Broadcast
	// descriptionPluginIDs is the plugin ID set for the cached descriptions.
	descriptionPluginIDs []string
	// descriptions caches plugin descriptions for the current plugin set.
	descriptions map[string]string
	// availablePluginManifestRefs fingerprints the manifest object content set for
	// the cached catalog.
	availablePluginManifestRefs []string
	// availablePlugins caches the installable plugin catalog for the current
	// manifest object content set.
	availablePlugins []*s4wave_space.AvailablePlugin
	// buildDescriptions overrides description lookup in tests.
	buildDescriptions func(context.Context, world.WorldState, []string) (map[string]string, error)
	// buildAvailablePlugins overrides catalog enumeration in tests.
	buildAvailablePlugins func(context.Context, world.WorldState) ([]*s4wave_space.AvailablePlugin, error)
	// lookupManifest overrides manifest lookup in tests.
	lookupManifest  func(context.Context, world.WorldState, string) (*bldr_manifest.Manifest, *bucket.ObjectRef, error)
	startController spaceContentsControllerStarter
}

// NewSpaceContentsResource creates a new SpaceContentsResource.
func NewSpaceContentsResource(le *logrus.Entry, b bus.Bus, engine world.Engine, spaceID, engineID string) *SpaceContentsResource {
	ctx, cancel := context.WithCancel(context.Background())
	r := &SpaceContentsResource{
		le:              le,
		b:               b,
		engine:          engine,
		spaceID:         spaceID,
		engineID:        engineID,
		ctx:             ctx,
		ctxCancel:       cancel,
		start:           newSpaceContentsStartRoutine(le),
		startController: startSpaceContentsController,
	}
	r.start.SetContext(ctx, false)
	mux := srpc.NewMux()
	_ = s4wave_space.SRPCRegisterSpaceContentsResourceService(mux, r)
	r.mux = mux
	return r
}

// Release releases the controller reference.
func (r *SpaceContentsResource) Release() {
	var start *routine.RoutineContainer
	var ref directive.Reference
	var cancel context.CancelFunc
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		r.released = true
		r.startSeq++
		start = r.start
		cancel = r.ctxCancel
		r.ctxCancel = nil
		r.ctx = nil
		ref = r.ctrlRef
		r.ctrlRef = nil
		r.ctrl = nil
		broadcast()
	})
	if cancel != nil {
		cancel()
	}
	if start != nil {
		start.ClearContext()
	}
	if ref != nil {
		ref.Release()
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
		storeID = process_binding.DefaultObjectStoreID
	}
	return volumeID, storeID
}

func (r *SpaceContentsResource) notifyController() {
	var ctrl *plugin_space.Controller
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ctrl = r.ctrl
	})
	if ctrl != nil {
		ctrl.NotifyChanged()
	}
}

// GetMux returns the rpc mux.
func (r *SpaceContentsResource) GetMux() srpc.Invoker {
	return r.mux
}

// StartController starts the plugin/space controller behind this resource.
func (r *SpaceContentsResource) StartController(conf *plugin_space.Config) {
	var seq uint64
	var start *routine.RoutineContainer
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.released {
			return
		}
		r.ensureStartOwnerLocked()
		r.startSeq++
		seq = r.startSeq
		start = r.start
		broadcast()
	})
	if start == nil {
		return
	}
	start.SetRoutine(func(ctx context.Context) error {
		return r.startControllerRoutine(ctx, seq, conf)
	})
}

func (r *SpaceContentsResource) ensureStartOwnerLocked() {
	if r.ctx == nil {
		r.ctx, r.ctxCancel = context.WithCancel(context.Background())
	}
	if r.start == nil {
		r.start = newSpaceContentsStartRoutine(r.le)
		r.start.SetContext(r.ctx, false)
	}
	if r.startController == nil {
		r.startController = startSpaceContentsController
	}
}

func (r *SpaceContentsResource) startControllerRoutine(ctx context.Context, seq uint64, conf *plugin_space.Config) error {
	ctrl, ctrlRef, err := r.startController(ctx, r.b, conf)
	if err != nil {
		if ctx.Err() == nil && r.le != nil {
			r.le.WithError(err).Warn("failed to start Space contents controller")
		}
		return nil
	}

	var release directive.Reference
	r.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if r.released || r.startSeq != seq || ctx.Err() != nil {
			release = ctrlRef
		} else {
			release = r.ctrlRef
			r.ctrl = ctrl
			r.ctrlRef = ctrlRef
		}
		broadcast()
	})
	if release != nil {
		release.Release()
	}
	return nil
}

func newSpaceContentsStartRoutine(le *logrus.Entry) *routine.RoutineContainer {
	if le == nil {
		return routine.NewRoutineContainer()
	}
	return routine.NewRoutineContainerWithLogger(le.WithField("routine", "space-contents-start"))
}

func startSpaceContentsController(
	ctx context.Context,
	b bus.Bus,
	conf *plugin_space.Config,
) (*plugin_space.Controller, directive.Reference, error) {
	ctrl, _, ctrlRef, err := plugin_space.StartControllerWithConfig(ctx, b, conf, func() {})
	return ctrl, ctrlRef, err
}

// WatchState streams the current plugin and process state for the space.
func (r *SpaceContentsResource) WatchState(
	req *s4wave_space.WatchSpaceContentsStateRequest,
	strm s4wave_space.SRPCSpaceContentsResourceService_WatchStateStream,
) error {
	ctx := strm.Context()

	var prevSeqno uint64
	for {
		// Read SpaceSettings, manifest descriptions, and the installable plugin
		// catalog from the world.
		var pluginIDs []string
		var descriptions map[string]string
		var availablePlugins []*s4wave_space.AvailablePlugin
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

			availablePlugins, err = r.getAvailablePlugins(ctx, wtx)
			if err != nil {
				r.le.WithError(err).Warn("failed to resolve plugin catalog")
				availablePlugins = nil
			}

			return nil
		}(); err != nil {
			return err
		}

		// Build plugin statuses.
		plugins := make([]*s4wave_space.SpacePluginStatus, 0, len(pluginIDs))
		loadedIDs := map[string]struct{}{}
		var loadedCh <-chan struct{}
		var waitStatusChange func(context.Context) error
		var ctrl *plugin_space.Controller
		r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			ctrl = r.ctrl
		})
		if ctrl != nil {
			var ids []string
			ids, loadedCh = ctrl.GetLoadedPluginIDsAndWaitCh()
			for _, pid := range ids {
				loadedIDs[pid] = struct{}{}
			}
		}
		schedulerStatuses := map[string]*bldr_plugin.PluginStatus{}
		if scheduler := plugin_host_scheduler.FindControllerOnBus(r.b); scheduler != nil {
			statusCtr := scheduler.GetPluginStatusCtr()
			statusSnapshot := statusCtr.GetValue()
			schedulerStatuses = spacePluginStatusesByID(statusSnapshot)
			waitStatusChange = func(waitCtx context.Context) error {
				_, err := statusCtr.WaitValueChange(waitCtx, statusSnapshot, nil)
				return err
			}
		}
		for _, pid := range pluginIDs {
			_, loaded := loadedIDs[pid]
			plugins = append(
				plugins,
				buildSpacePluginStatus(
					pid,
					descriptions[pid],
					loaded,
					ctrl != nil,
					schedulerStatuses[pid],
				),
			)
		}
		processBindings, err := r.listProcessBindingInfos(ctx)
		if err != nil {
			return err
		}

		if err := strm.Send(&s4wave_space.SpaceContentsState{
			Ready:            true,
			Plugins:          plugins,
			ProcessBindings:  processBindings,
			AvailablePlugins: availablePlugins,
		}); err != nil {
			return err
		}

		// Wait for world seqno change, process-binding change, or loaded state change.
		var ch <-chan struct{}
		r.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
		})
		err = waitSpaceContentsSources(ctx, func(waitCtx context.Context) error {
			_, err := r.engine.WaitSeqno(waitCtx, prevSeqno+1)
			return err
		}, []<-chan struct{}{ch, loadedCh}, waitStatusChange)
		if err != nil {
			return err
		}
	}
}

func waitSpaceContentsSeqno(
	ctx context.Context,
	waitSeqno func(context.Context) error,
	waitChs ...<-chan struct{},
) error {
	return waitSpaceContentsSources(ctx, waitSeqno, waitChs, nil)
}

func waitSpaceContentsSources(
	ctx context.Context,
	waitSeqno func(context.Context) error,
	waitChs []<-chan struct{},
	waitFns ...func(context.Context) error,
) error {
	waitCtx, waitCancel := context.WithCancel(ctx)
	defer waitCancel()

	waitCount := 1
	for _, waitFn := range waitFns {
		if waitFn != nil {
			waitCount++
		}
	}
	waitAnyDone := make(chan struct{}, waitCount)
	go func() {
		if err := broadcast.WaitAny(waitCtx, waitChs...); err == nil {
			waitCancel()
		}
		waitAnyDone <- struct{}{}
	}()
	for _, waitFn := range waitFns {
		if waitFn == nil {
			continue
		}
		go func() {
			if err := waitFn(waitCtx); err == nil {
				waitCancel()
			}
			waitAnyDone <- struct{}{}
		}()
	}

	err := waitSeqno(waitCtx)
	waitCancel()
	for range waitCount {
		<-waitAnyDone
	}
	if err != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return nil
}

func spacePluginStatusesByID(
	snapshot *plugin_host_scheduler.PluginStatusSnapshot,
) map[string]*bldr_plugin.PluginStatus {
	statuses := map[string]*bldr_plugin.PluginStatus{}
	if snapshot == nil {
		return statuses
	}
	for _, plugin := range snapshot.Plugins {
		if plugin == nil || plugin.GetInstanceKey() != "" {
			continue
		}
		statuses[plugin.GetPluginId()] = plugin
	}
	return statuses
}

func buildSpacePluginStatus(
	pluginID string,
	description string,
	loaded bool,
	controllerStarted bool,
	schedulerStatus *bldr_plugin.PluginStatus,
) *s4wave_space.SpacePluginStatus {
	state := s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_CONFIGURED
	detail := ""
	if controllerStarted {
		state = s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADING
		detail = "Plugin runtime requested"
	}
	if schedulerStatus != nil {
		state, detail = projectSpacePluginLifecycle(schedulerStatus)
	}
	if loaded || (schedulerStatus != nil && schedulerStatus.GetRunning()) {
		loaded = true
		state = s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADED
		detail = ""
	}
	return &s4wave_space.SpacePluginStatus{
		PluginId:    pluginID,
		Loaded:      loaded,
		Description: description,
		State:       state,
		Detail:      detail,
	}
}

func projectSpacePluginLifecycle(
	status *bldr_plugin.PluginStatus,
) (s4wave_space.SpacePluginLifecycleState, string) {
	if msg := status.GetLastErrorMessage(); msg != "" {
		if status.GetState() == bldr_plugin.PluginState_PluginState_REQUESTED {
			return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_RETRYING, msg
		}
		return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED, msg
	}
	switch status.GetState() {
	case bldr_plugin.PluginState_PluginState_RUNNING:
		return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADED, ""
	case bldr_plugin.PluginState_PluginState_REQUESTED:
		return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADING, "Plugin runtime requested"
	default:
		return s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_CONFIGURED, ""
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

// getAvailablePlugins returns the cached installable plugin catalog for the
// current manifest object content set, using the test override when set.
func (r *SpaceContentsResource) getAvailablePlugins(
	ctx context.Context,
	ws world.WorldState,
) ([]*s4wave_space.AvailablePlugin, error) {
	if r.buildAvailablePlugins != nil {
		return r.buildAvailablePlugins(ctx, ws)
	}

	manifestRefs, err := collectAvailablePluginManifestRefs(ctx, ws)
	if err != nil {
		return nil, err
	}

	var cached []*s4wave_space.AvailablePlugin
	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if slices.Equal(r.availablePluginManifestRefs, manifestRefs) {
			cached = cloneAvailablePlugins(r.availablePlugins)
		}
	})
	if cached != nil {
		return cached, nil
	}

	availablePlugins, err := r.collectAvailablePlugins(ctx, ws, manifestRefs)
	if err != nil {
		return nil, err
	}

	r.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		r.availablePluginManifestRefs = slices.Clone(manifestRefs)
		r.availablePlugins = cloneAvailablePlugins(availablePlugins)
	})
	return cloneAvailablePlugins(availablePlugins), nil
}

func collectAvailablePluginManifestRefs(
	ctx context.Context,
	ws world.WorldState,
) ([]string, error) {
	manifestKeys, err := world_types.ListObjectsWithType(ctx, ws, bldr_manifest_world.ManifestTypeID)
	if err != nil {
		return nil, err
	}
	manifestKeys = slices.Clone(manifestKeys)
	slices.Sort(manifestKeys)

	manifestRefs := make([]string, 0, len(manifestKeys))
	for _, key := range manifestKeys {
		obj, err := world.MustGetObject(ctx, ws, key)
		if err != nil {
			return nil, err
		}
		ref, _, err := obj.GetRootRef(ctx)
		if err != nil {
			return nil, err
		}
		manifestRefs = append(manifestRefs, key+"\x00"+ref.MarshalString())
	}
	return manifestRefs, nil
}

// collectAvailablePlugins enumerates the manifest object content set and returns
// the installable plugin catalog, keeping the highest revision for each manifest
// ID.
func (r *SpaceContentsResource) collectAvailablePlugins(
	ctx context.Context,
	ws world.WorldState,
	manifestRefs []string,
) ([]*s4wave_space.AvailablePlugin, error) {
	lookupManifest := r.lookupManifest
	if lookupManifest == nil {
		lookupManifest = bldr_manifest_world.LookupManifest
	}

	catalog := make(map[string]*bldr_manifest.ManifestMeta, len(manifestRefs))
	for _, ref := range manifestRefs {
		key, _, _ := strings.Cut(ref, "\x00")
		m, _, err := lookupManifest(ctx, ws, key)
		if err != nil || m == nil {
			continue
		}
		addManifestToCatalog(catalog, m.GetMeta())
	}

	return availablePluginsFromCatalog(catalog), nil
}

// addManifestToCatalog records the manifest meta under its manifest ID, keeping
// the highest revision when the same plugin has multiple platform/revision
// builds.
func addManifestToCatalog(catalog map[string]*bldr_manifest.ManifestMeta, meta *bldr_manifest.ManifestMeta) {
	manifestID := meta.GetManifestId()
	if manifestID == "" {
		return
	}
	if prev, ok := catalog[manifestID]; !ok || meta.GetRev() > prev.GetRev() {
		catalog[manifestID] = meta
	}
}

// availablePluginsFromCatalog projects the collected catalog into the sorted
// app-facing available plugin list.
func availablePluginsFromCatalog(catalog map[string]*bldr_manifest.ManifestMeta) []*s4wave_space.AvailablePlugin {
	out := make([]*s4wave_space.AvailablePlugin, 0, len(catalog))
	for manifestID, meta := range catalog {
		out = append(out, &s4wave_space.AvailablePlugin{
			PluginId:    manifestID,
			Description: meta.GetDescription(),
			Revision:    strconv.FormatUint(meta.GetRev(), 10),
		})
	}
	slices.SortFunc(out, func(a, b *s4wave_space.AvailablePlugin) int {
		return cmp.Compare(a.GetPluginId(), b.GetPluginId())
	})
	return out
}

func cloneAvailablePlugins(in []*s4wave_space.AvailablePlugin) []*s4wave_space.AvailablePlugin {
	if in == nil {
		return nil
	}
	out := make([]*s4wave_space.AvailablePlugin, 0, len(in))
	for _, plugin := range in {
		out = append(out, plugin.CloneVT())
	}
	return out
}

// SetProcessBinding sets the state for a process binding.
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
var _ s4wave_space.SRPCSpaceContentsResourceServiceServer = (*SpaceContentsResource)(nil)
