package resource_space

import (
	"cmp"
	"context"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	process_binding "github.com/s4wave/spacewave/core/plugin/process"
	plugin_space_runtime "github.com/s4wave/spacewave/core/plugin/space/runtime"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
	s4wave_process "github.com/s4wave/spacewave/sdk/process"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/sirupsen/logrus"
)

// errSpaceContentsReleased reports a call on a released contents mount.
var errSpaceContentsReleased = errors.New("space contents resource is released")

// SpaceContentsResource provides streaming plugin status for one contents
// mount of a Space. Every mount of the Space shares one plugin runtime.
type SpaceContentsResource struct {
	le       *logrus.Entry
	b        bus.Bus
	mux      srpc.Invoker
	engine   world.Engine
	spaceID  string
	engineID string
	volumeID string
	storeID  string
	// runtime is the shared plugin runtime of the Space.
	runtime *plugin_space_runtime.Controller
	// runtimeRef holds this mount's reference to runtime.
	runtimeRef directive.Reference
	// ctx is canceled when the mount is released.
	ctx       context.Context
	ctxCancel context.CancelFunc
	// mu guards the cached plugin descriptions and manifest catalog below.
	mu sync.Mutex
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
	lookupManifest func(context.Context, world.WorldState, string) (*bldr_manifest.Manifest, *bucket.ObjectRef, error)
	// afterAttachedRpcServiceReady blocks the bind continuation in focused lifecycle tests.
	afterAttachedRpcServiceReady func()
}

// NewSpaceContentsResource creates a contents mount of the Space runtime.
// The mount owns runtimeRef and releases it in Release.
func NewSpaceContentsResource(
	le *logrus.Entry,
	b bus.Bus,
	engine world.Engine,
	spaceID string,
	engineID string,
	runtime *plugin_space_runtime.Controller,
	runtimeRef directive.Reference,
) *SpaceContentsResource {
	ctx, cancel := context.WithCancel(context.Background()) //nolint:gosec // SpaceContentsResource.Release owns the stored cancellation function.
	r := &SpaceContentsResource{
		le:         le,
		b:          b,
		engine:     engine,
		spaceID:    spaceID,
		engineID:   engineID,
		runtime:    runtime,
		runtimeRef: runtimeRef,
		ctx:        ctx,
		ctxCancel:  cancel,
	}
	mux := srpc.NewMux()
	_ = s4wave_space.SRPCRegisterSpaceContentsResourceService(mux, r)
	r.mux = mux
	return r
}

// Release ends the calls of the mount and drops its runtime reference. The
// runtime stops after the last mount of the Space is released.
func (r *SpaceContentsResource) Release() {
	r.ctxCancel()
	r.runtimeRef.Release()
}

// getStoreLocation returns the volume and object store IDs that hold the
// process bindings, falling back to the plugin volume defaults.
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

// GetMux returns the rpc mux.
func (r *SpaceContentsResource) GetMux() srpc.Invoker {
	return r.mux
}

// BindAttachedRpcService publishes a caller-attached Resource under one private
// service ID prefix for the caller attachment lifetime. The route follows the
// running generation of the shared Space runtime, and the prefix is reserved
// across every mount of the Space.
func (r *SpaceContentsResource) BindAttachedRpcService(
	req *s4wave_space.BindAttachedRpcServiceRequest,
	strm s4wave_space.SRPCSpaceContentsResourceService_BindAttachedRpcServiceStream,
) error {
	if req.GetAttachedResourceId() == 0 {
		return errors.New("attached resource ID must be nonzero")
	}
	prefix := req.GetServiceIdPrefix()
	if !isSafeAttachedRpcServicePrefix(prefix) {
		return errors.New("service ID prefix must be nonempty and safe")
	}

	resourceCtx, err := resource_server.MustGetResourceClientContext(strm.Context())
	if err != nil {
		return err
	}
	client, err := resourceCtx.GetAttachedResource(req.GetAttachedResourceId())
	if err != nil {
		return err
	}
	var clientDone <-chan struct{}
	if doneClient, ok := client.(interface{ Done() <-chan struct{} }); ok {
		clientDone = doneClient.Done()
	}

	// Refuse a route when the caller, attachment, request, or mount has already
	// ended.
	if err := r.attachedRpcServiceLifetimeError(strm.Context(), resourceCtx.Context(), clientDone); err != nil {
		return err
	}
	releasePrefix, err := r.runtime.ReserveServicePrefix(prefix)
	if err != nil {
		return err
	}
	defer releasePrefix()

	invoker := srpc.NewClientInvoker(client)
	var installed *plugin_space_runtime.Generation
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	ready := false
	for {
		gen, waitCh, err := r.runtime.GetGeneration()
		if err != nil {
			return err
		}
		if gen == nil {
			select {
			case <-strm.Context().Done():
				return strm.Context().Err()
			case <-resourceCtx.Context().Done():
				return resourceCtx.Context().Err()
			case <-r.ctx.Done():
				return errSpaceContentsReleased
			case <-clientDone:
				return nil
			case <-waitCh:
				continue
			}
		}
		if gen == installed {
			select {
			case <-strm.Context().Done():
				return strm.Context().Err()
			case <-resourceCtx.Context().Done():
				return resourceCtx.Context().Err()
			case <-r.ctx.Done():
				return errSpaceContentsReleased
			case <-clientDone:
				return nil
			case <-gen.Done():
				installed = nil
				release()
				release = nil
				continue
			}
		}

		// Stop the previous generation's route before installing it on the
		// replacement.
		if release != nil {
			release()
			release = nil
		}
		ctrl := NewAttachedRpcServiceController(prefix, invoker)
		release, err = gen.GetBus().AddController(strm.Context(), ctrl, nil)
		if err != nil {
			select {
			case <-gen.Done():
				continue
			default:
			}
			return err
		}
		installed = gen

		// Wait until the route can answer before the first readiness response.
		select {
		case <-ctrl.Ready():
		case <-strm.Context().Done():
			return strm.Context().Err()
		case <-resourceCtx.Context().Done():
			return resourceCtx.Context().Err()
		case <-r.ctx.Done():
			return errSpaceContentsReleased
		case <-clientDone:
			return errors.New("attached resource ended before attached service was ready")
		case <-gen.Done():
			installed = nil
			release()
			release = nil
			continue
		}
		// Let focused lifecycle tests replace the Space runtime after its route is ready.
		if r.afterAttachedRpcServiceReady != nil {
			r.afterAttachedRpcServiceReady()
		}

		// Publish this route only while its generation is still running.
		current, _, err := r.runtime.GetGeneration()
		if err != nil {
			return err
		}
		if current != gen {
			installed = nil
			release()
			release = nil
			continue
		}
		if !ready {
			// Check the caller lifetimes again before reporting the first route.
			if err := r.attachedRpcServiceLifetimeError(strm.Context(), resourceCtx.Context(), clientDone); err != nil {
				return err
			}
			if err := strm.Send(&s4wave_space.BindAttachedRpcServiceResponse{}); err != nil {
				return err
			}
			ready = true
		}
	}
}

// attachedRpcServiceLifetimeError reports an ended binding lifetime without waiting.
func (r *SpaceContentsResource) attachedRpcServiceLifetimeError(
	streamCtx context.Context,
	resourceCtx context.Context,
	clientDone <-chan struct{},
) error {
	if err := streamCtx.Err(); err != nil {
		return err
	}
	if err := resourceCtx.Err(); err != nil {
		return err
	}
	if r.ctx.Err() != nil {
		return errSpaceContentsReleased
	}
	select {
	case <-clientDone:
		return errors.New("attached resource ended before attached service was ready")
	default:
	}
	return nil
}

// isSafeAttachedRpcServicePrefix reports whether prefix is a bounded, relative
// service ID prefix ending in a slash with no whitespace or control characters.
func isSafeAttachedRpcServicePrefix(prefix string) bool {
	if prefix == "" || len(prefix) > 256 || !utf8.ValidString(prefix) ||
		prefix[0] == '/' || prefix[len(prefix)-1] != '/' {
		return false
	}
	for _, char := range prefix {
		if unicode.IsSpace(char) || unicode.IsControl(char) {
			return false
		}
	}
	return true
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

			settings, err := space_world.LookupSpaceSettingsBody(ctx, wtx)
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

		// Build plugin statuses from the running runtime generation.
		gen, runtimeCh, runtimeErr := r.runtime.GetGeneration()
		loadedIDs := map[string]struct{}{}
		var loadedCh <-chan struct{}
		schedulerStatuses := map[string]*bldr_plugin.PluginStatus{}
		var waitStatusChange func(context.Context) error
		if gen != nil {
			var ids []string
			ids, loadedCh = gen.GetSpaceController().GetLoadedPluginIDsAndWaitCh()
			for _, pid := range ids {
				loadedIDs[pid] = struct{}{}
			}

			statusCtr := gen.GetScheduler().GetPluginStatusCtr()
			statusSnapshot := statusCtr.GetValue()
			schedulerStatuses = spacePluginStatusesByID(statusSnapshot, r.spaceID)
			waitStatusChange = func(waitCtx context.Context) error {
				_, err := statusCtr.WaitValueChange(waitCtx, statusSnapshot, nil)
				return err
			}
		}
		plugins := make([]*s4wave_space.SpacePluginStatus, 0, len(pluginIDs))
		for _, pid := range pluginIDs {
			_, loaded := loadedIDs[pid]
			status := buildSpacePluginStatus(
				pid,
				descriptions[pid],
				loaded,
				gen != nil,
				schedulerStatuses[pid],
			)
			if runtimeErr != nil {
				status.State = s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED
				status.Detail = runtimeErr.Error()
			}
			plugins = append(plugins, status)
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

		// Wait for a world seqno, runtime, process binding, or loaded state change.
		err = waitSpaceContentsSources(ctx, func(waitCtx context.Context) error {
			_, err := r.engine.WaitSeqno(waitCtx, prevSeqno+1)
			return err
		}, []<-chan struct{}{runtimeCh, loadedCh}, waitStatusChange)
		if err != nil {
			return err
		}
	}
}

// waitSpaceContentsSources blocks until waitSeqno returns, any channel in
// waitChs closes, or any waitFn returns nil. It returns ctx.Err when ctx ends
// first and nil after any wake.
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

// spacePluginStatusesByID indexes the scheduler statuses of instanceKey by
// plugin ID.
func spacePluginStatusesByID(
	snapshot *bldr_plugin.PluginStatusSnapshot,
	instanceKey string,
) map[string]*bldr_plugin.PluginStatus {
	statuses := map[string]*bldr_plugin.PluginStatus{}
	if snapshot == nil {
		return statuses
	}
	for _, plugin := range snapshot.Plugins {
		if plugin == nil || plugin.GetInstanceKey() != instanceKey {
			continue
		}
		statuses[plugin.GetPluginId()] = plugin
	}
	return statuses
}

// buildSpacePluginStatus projects the loaded state and scheduler status of one
// configured plugin into its Space lifecycle status.
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

// projectSpacePluginLifecycle maps a scheduler plugin status to a Space
// lifecycle state and detail message.
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
	r.mu.Lock()
	if slices.Equal(r.descriptionPluginIDs, pluginIDs) {
		cached = maps.Clone(r.descriptions)
	}
	r.mu.Unlock()
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

	r.mu.Lock()
	r.descriptionPluginIDs = slices.Clone(pluginIDs)
	r.descriptions = maps.Clone(descriptions)
	r.mu.Unlock()
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
	r.mu.Lock()
	if slices.Equal(r.availablePluginManifestRefs, manifestRefs) {
		cached = cloneAvailablePlugins(r.availablePlugins)
	}
	r.mu.Unlock()
	if cached != nil {
		return cached, nil
	}

	availablePlugins, err := r.collectAvailablePlugins(ctx, ws, manifestRefs)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.availablePluginManifestRefs = slices.Clone(manifestRefs)
	r.availablePlugins = cloneAvailablePlugins(availablePlugins)
	r.mu.Unlock()
	return cloneAvailablePlugins(availablePlugins), nil
}

// collectAvailablePluginManifestRefs returns the root refs of every manifest
// object in key order, fingerprinting the installable plugin catalog.
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
		world.ReleaseObjectState(obj)
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

// cloneAvailablePlugins deep-copies the plugin catalog so callers cannot
// mutate the cache.
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

	r.runtime.NotifyProcessBindingsChanged()
	return &s4wave_space.SetProcessBindingResponse{}, nil
}

// listProcessBindingInfos returns the process bindings of the Space from the
// peer-local object store.
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
