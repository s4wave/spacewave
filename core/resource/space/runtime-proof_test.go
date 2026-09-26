package resource_space

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_entrypoint_controller "github.com/s4wave/spacewave/bldr/plugin/entrypoint/controller"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_mock "github.com/s4wave/spacewave/bldr/plugin/host/mock"
	plugin_host_static "github.com/s4wave/spacewave/bldr/plugin/host/static"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// spaceRuntimeManifestID is the plugin approved by the runtime tests.
const spaceRuntimeManifestID = "cold-plugin"

func TestSpaceRuntimeSchedulesApprovedPluginFromParentManifestSource(t *testing.T) {
	ctx, tb := newSpaceRuntimeTestbed(t)
	parentLoads := newLoadPluginRecorder()
	addSpaceRuntimeController(t, tb.Bus, parentLoads)
	addSpaceRuntimePluginHost(t, tb.Bus, "test/platform")
	manifestSource := newEmptyManifestSource(spaceRuntimeManifestID)
	addSpaceRuntimeController(t, tb.Bus, manifestSource)

	resource := newTestSpaceContentsResource(t, tb.Logger, tb.Bus, tb.Engine, newSpaceRuntimeConfig(tb))
	gen := waitSpaceRuntimeGeneration(t, resource.runtime, nil)
	select {
	case <-manifestSource.started:
		t.Fatal("unapproved Space plugin fetched its parent manifest")
	default:
	}

	approveSpaceRuntimePlugin(t, ctx, tb)
	select {
	case <-manifestSource.started:
	case <-ctx.Done():
		t.Fatal("approved Space plugin did not fetch its parent manifest")
	}

	scheduler := gen.GetScheduler()
	statuses := scheduler.GetPluginStatusCtr().GetValue()
	if !slices.ContainsFunc(statuses.GetPlugins(), func(status *bldr_plugin.PluginStatus) bool {
		return status.GetPluginId() == spaceRuntimeManifestID &&
			status.GetInstanceKey() == "space-test" &&
			status.GetState() == bldr_plugin.PluginState_PluginState_REQUESTED
	}) {
		t.Fatalf("plugin lifecycle did not reach requested: %#v", statuses)
	}

	// Session status finds the Space scheduler from the parent bus.
	schedulers, _, schedulersRef, err := bus.ExecCollectValues[bldr_plugin.LookupPluginSchedulerValue](
		ctx,
		tb.Bus,
		bldr_plugin.NewLookupPluginScheduler(),
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	schedulersRef.Release()
	if !slices.ContainsFunc(schedulers, func(s bldr_plugin.PluginScheduler) bool {
		return s == scheduler && s.GetInstanceKey() == "space-test"
	}) {
		t.Fatalf("parent bus lookup did not reach the Space scheduler: %#v", schedulers)
	}
	select {
	case load := <-parentLoads.loads:
		t.Fatalf("empty HostPluginId leaked LoadPlugin to parent: %#v", load)
	default:
	}
}

func TestSpaceRuntimeRoutesPluginHostLoadToParentEntrypoint(t *testing.T) {
	ctx, tb := newSpaceRuntimeTestbed(t)
	host := newRecordingPluginHostServer()
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(mux, host); err != nil {
		t.Fatal(err)
	}
	addSpaceRuntimeController(t, tb.Bus, plugin_entrypoint_controller.NewController(
		tb.Bus,
		tb.Logger,
		&bldr_plugin.PluginMeta{PluginId: "spacewave-core"},
		bldr_plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))),
	))

	conf := newSpaceRuntimeConfig(tb)
	conf.HostPluginId = "spacewave-core"
	resource := newTestSpaceContentsResource(t, tb.Logger, tb.Bus, tb.Engine, conf)
	resource.volumeID = tb.EngineVolumeID
	resource.storeID = tb.EngineObjectStoreID
	waitSpaceRuntimeGeneration(t, resource.runtime, nil)

	approveSpaceRuntimePlugin(t, ctx, tb)
	select {
	case req := <-host.requests:
		if req.GetPluginId() != spaceRuntimeManifestID || req.GetInstanceKey() != "space-test" {
			t.Fatalf("LoadPlugin request = %#v", req)
		}
	case <-ctx.Done():
		t.Fatal("parent plugin entrypoint did not receive LoadPlugin")
	}

	// Receiving LoadPlugin does not mean its response has reached every status
	// observer. Accept intermediate snapshots until the plugin reports loaded.
	watchSpaceRuntimeState(t, ctx, resource, func(state *s4wave_space.SpaceContentsState) bool {
		if len(state.GetPlugins()) != 1 {
			t.Fatalf("plugin count = %d, want 1", len(state.GetPlugins()))
		}
		status := state.GetPlugins()[0]
		if status.GetState() == s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED {
			t.Fatalf("plugin failed: %s", status.GetDetail())
		}
		return status.GetLoaded() &&
			status.GetState() == s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADED
	})

	// A Space-stored installation crosses the parent RPC with its exact artifact,
	// even when that artifact is absent from the parent's application catalog.
	selected := createSpacePluginManifest(t, ctx, tb, spaceRuntimeManifestID, "js", 7)
	key := bldr_manifest.NewManifestArtifactKey(selected.GetManifestRef())
	if _, _, err := manifest_world.SetManifest(ctx, tb.WorldState, "", key, selected.GetManifestRef()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := space_world_ops.SetSpaceSettings(ctx, tb.WorldState, "", "", &space_world.SpaceSettings{
		PluginIds: []string{spaceRuntimeManifestID},
		PluginInstallations: map[string]*space_world.SpacePluginInstallation{
			spaceRuntimeManifestID: {ManifestKeys: []string{key}},
		},
	}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	select {
	case req := <-host.requests:
		if req.GetPluginId() != spaceRuntimeManifestID || req.GetInstanceKey() != "space-test" ||
			!req.GetManifests()[0].GetManifestRef().GetRootRef().EqualVT(selected.GetManifestRef().GetRootRef()) {
			t.Fatalf("selected LoadPlugin request lost installation identity: %#v", req)
		}
	case <-ctx.Done():
		t.Fatal("selected artifact did not reach the parent plugin host")
	}

	// Releasing the only mount stops the runtime and its parent load.
	resource.Release()
	select {
	case <-host.released:
	case <-ctx.Done():
		t.Fatal("releasing Space did not cancel parent LoadPlugin stream")
	}
}

func TestSpaceRuntimeRestartsAfterParentHostPublication(t *testing.T) {
	ctx, tb := newSpaceRuntimeTestbed(t)
	manifestSource := newEmptyManifestSource(spaceRuntimeManifestID)
	addSpaceRuntimeController(t, tb.Bus, manifestSource)

	resource := newTestSpaceContentsResource(t, tb.Logger, tb.Bus, tb.Engine, newSpaceRuntimeConfig(tb))
	first := waitSpaceRuntimeGeneration(t, resource.runtime, nil)

	addSpaceRuntimePluginHost(t, tb.Bus, "test/platform")
	waitSpaceRuntimeGeneration(t, resource.runtime, first)
	approveSpaceRuntimePlugin(t, ctx, tb)
	select {
	case <-manifestSource.started:
	case <-ctx.Done():
		t.Fatal("restarted Space runtime did not fetch the approved parent manifest")
	}
}

func TestBindAttachedRpcServiceRebindsAfterSpaceRuntimeReplacement(t *testing.T) {
	ctx, tb := newSpaceRuntimeTestbed(t)
	addSpaceRuntimeController(t, tb.Bus, newEmptyManifestSource(spaceRuntimeManifestID))

	resource := newTestSpaceContentsResource(t, tb.Logger, tb.Bus, tb.Engine, newSpaceRuntimeConfig(tb))
	first := waitSpaceRuntimeGeneration(t, resource.runtime, nil)
	invokeCurrent := func(body string) error {
		gen, _, err := resource.runtime.GetGeneration()
		if err != nil {
			return err
		}
		if gen == nil {
			return errors.New("Space runtime is not running")
		}
		return invokeAttachedEcho(ctx, gen.GetBus(), body)
	}

	// Pause after the old route becomes callable, before it can publish readiness.
	readyEntered := make(chan struct{})
	continueBind := make(chan struct{})
	var readyOnce sync.Once
	resource.afterAttachedRpcServiceReady = func() {
		readyOnce.Do(func() {
			close(readyEntered)
			<-continueBind
		})
	}

	attachedResources := newSpaceRecordingResourceClient(ctx)
	attachedID := addAttachedEchoResource(t, attachedResources)
	stream := newAttachedRpcServiceStream(resource_server.WithResourceClientContext(ctx, attachedResources))
	stream.checkReady = func() error {
		return invokeCurrent("first response")
	}
	bindDone := make(chan error, 1)
	go func() {
		bindDone <- resource.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
			AttachedResourceId: attachedID,
			ServiceIdPrefix:    "attached/",
		}, stream)
	}()
	select {
	case <-readyEntered:
	case <-ctx.Done():
		t.Fatal("attached route did not become callable")
	}
	if err := invokeCurrent("before replacement"); err != nil {
		t.Fatal(err)
	}

	// Replace the generation while the old bind continuation is paused.
	addSpaceRuntimePluginHost(t, tb.Bus, "test/platform")
	second := waitSpaceRuntimeGeneration(t, resource.runtime, first)
	select {
	case <-stream.ready:
		t.Fatal("old generation reported readiness after replacement")
	default:
	}

	close(continueBind)
	select {
	case <-stream.ready:
	case <-ctx.Done():
		t.Fatal("replacement attached route did not become ready")
	}
	if err := invokeCurrent("after replacement"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-bindDone:
		t.Fatalf("bind stream ended after generation replacement: %v", err)
	default:
	}

	if !attachedResources.ReleaseResource(attachedID) {
		t.Fatal("attached resource release failed")
	}
	if err := <-bindDone; err != nil {
		t.Fatalf("bind returned %v after attached resource release", err)
	}
	if countAttachedEcho(t, ctx, second.GetBus()) != 0 {
		t.Fatal("released attachment remained callable")
	}
}

func TestSpaceContentsMountsShareRuntime(t *testing.T) {
	ctx, tb := newSpaceRuntimeTestbed(t)
	conf := newSpaceRuntimeConfig(tb)
	first := newTestSpaceContentsResource(t, tb.Logger, tb.Bus, tb.Engine, conf)
	second := newTestSpaceContentsResource(t, tb.Logger, tb.Bus, tb.Engine, conf.CloneVT())
	if first.runtime != second.runtime {
		t.Fatal("two mounts of one Space started two runtimes")
	}
	for _, mount := range []*SpaceContentsResource{first, second} {
		mount.volumeID = tb.EngineVolumeID
		mount.storeID = tb.EngineObjectStoreID
	}
	gen := waitSpaceRuntimeGeneration(t, first.runtime, nil)

	// A process binding decided through one mount reaches the other.
	if _, err := first.SetProcessBinding(ctx, &s4wave_space.SetProcessBindingRequest{
		ObjectKey: "object",
		TypeId:    "test/type",
	}); err != nil {
		t.Fatal(err)
	}
	watchSpaceRuntimeState(t, ctx, second, func(state *s4wave_space.SpaceContentsState) bool {
		return len(state.GetProcessBindings()) == 1
	})

	// An attached service prefix bound through one mount is refused on the other.
	attachedResources := newSpaceRecordingResourceClient(ctx)
	attachedID := addAttachedEchoResource(t, attachedResources)
	bindCtx := resource_server.WithResourceClientContext(ctx, attachedResources)
	stream := newAttachedRpcServiceStream(bindCtx)
	bindDone := make(chan error, 1)
	go func() {
		bindDone <- first.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
			AttachedResourceId: attachedID,
			ServiceIdPrefix:    "attached/",
		}, stream)
	}()
	<-stream.ready
	if err := second.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
		AttachedResourceId: attachedID,
		ServiceIdPrefix:    "attached/",
	}, newAttachedRpcServiceStream(bindCtx)); err == nil {
		t.Fatal("second mount rebound a bound prefix")
	}

	// Releasing the first mount ends its calls and keeps the runtime.
	first.Release()
	if err := <-bindDone; !errors.Is(err, errSpaceContentsReleased) {
		t.Fatalf("bind on released mount returned %v", err)
	}
	select {
	case <-gen.Done():
		t.Fatal("releasing one of two mounts stopped the runtime")
	case <-time.After(50 * time.Millisecond):
	}
	second.Release()
	select {
	case <-gen.Done():
	case <-ctx.Done():
		t.Fatal("releasing the last mount did not stop the runtime")
	}
}

func TestSpaceContentsResourceProjectsPluginHostWatchChange(t *testing.T) {
	ctx, tb := newSpaceRuntimeTestbed(t)
	setSpaceRuntimeTestPlugin(t, ctx, tb)
	addSpaceRuntimePluginHost(t, tb.Bus, "desktop/test-a")

	resource := newTestSpaceContentsResource(t, tb.Logger, tb.Bus, tb.Engine, &plugin_space.Config{
		SpaceId:       "space-test",
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	})
	resource.volumeID = tb.EngineVolumeID
	first := waitSpaceRuntimeGeneration(t, resource.runtime, nil)

	stream, stop := startSpaceRuntimeWatch(t, ctx, resource)
	defer stop()
	recvSpaceRuntimeWatchState(t, stream)

	addSpaceRuntimePluginHost(t, tb.Bus, "desktop/test-b")
	waitSpaceRuntimeGeneration(t, resource.runtime, first)
	state := recvSpaceRuntimeWatchState(t, stream)
	if len(state.GetPlugins()) != 1 {
		t.Fatalf("plugins = %d, want 1", len(state.GetPlugins()))
	}
	if state.GetPlugins()[0].GetState() == s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED {
		t.Fatalf("plugin state = %#v", state.GetPlugins()[0])
	}
}

func TestSpaceContentsResourceProjectsPluginHostWatchError(t *testing.T) {
	ctx, tb := newSpaceRuntimeTestbed(t)
	setSpaceRuntimeTestPlugin(t, ctx, tb)
	addSpaceRuntimePluginHost(t, tb.Bus, "desktop/test-a")

	resource := newTestSpaceContentsResource(t, tb.Logger, tb.Bus, tb.Engine, &plugin_space.Config{
		SpaceId:       "space-test",
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	})
	resource.volumeID = tb.EngineVolumeID
	first := waitSpaceRuntimeGeneration(t, resource.runtime, nil)

	stream, stop := startSpaceRuntimeWatch(t, ctx, resource)
	defer stop()
	recvSpaceRuntimeWatchState(t, stream)

	addSpaceRuntimeController(
		t,
		tb.Bus,
		plugin_host_mock.NewLookupErrorController(errors.New("test plugin host watch error")),
	)
	for {
		plugin := recvSpaceRuntimeWatchState(t, stream).GetPlugins()[0]
		if plugin.GetState() != s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED {
			continue
		}
		if plugin.GetDetail() != "watch daemon plugin hosts: test plugin host watch error" {
			t.Fatalf("plugin detail = %q", plugin.GetDetail())
		}
		break
	}
	select {
	case <-first.Done():
	case <-ctx.Done():
		t.Fatal("failed generation was not released")
	}
}

// newSpaceRuntimeTestbed starts a testbed that stops when the test ends. Its
// volume also serves as the plugin volume. The returned context bounds the
// test.
func newSpaceRuntimeTestbed(t *testing.T) (context.Context, *testbed.Testbed) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	t.Cleanup(cancel)
	tb, err := testbed.WithTestbedOptions(ctx, []db_testbed.Option{
		db_testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
			VolumeConfig: &volume_controller.Config{
				VolumeIdAlias: []string{bldr_plugin.PluginVolumeID},
			},
		}),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	return ctx, tb
}

// newSpaceRuntimeConfig returns the plugin/space config of "space-test" on the
// testbed engine.
func newSpaceRuntimeConfig(tb *testbed.Testbed) *plugin_space.Config {
	return &plugin_space.Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	}
}

// approveSpaceRuntimePlugin approves the cold test plugin in the Space settings.
func approveSpaceRuntimePlugin(t *testing.T, ctx context.Context, tb *testbed.Testbed) {
	t.Helper()
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		peer.ID(""),
		"",
		&space_world.SpaceSettings{PluginIds: []string{spaceRuntimeManifestID}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
}

// setSpaceRuntimeTestPlugin approves one plugin that no host can run.
func setSpaceRuntimeTestPlugin(t *testing.T, ctx context.Context, tb *testbed.Testbed) {
	t.Helper()
	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx,
		tb.WorldState,
		peer.ID(""),
		"",
		&space_world.SpaceSettings{PluginIds: []string{"test-plugin"}},
		true,
		time.Now(),
	); err != nil {
		t.Fatal(err)
	}
}

// addSpaceRuntimeController adds ctrl to b until the test ends.
func addSpaceRuntimeController(t *testing.T, b bus.Bus, ctrl controller.Controller) {
	t.Helper()
	release, err := b.AddController(t.Context(), ctrl, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
}

// addSpaceRuntimePluginHost publishes a daemon plugin host until the test ends.
func addSpaceRuntimePluginHost(t *testing.T, b bus.Bus, platformID string) {
	t.Helper()
	addSpaceRuntimeController(t, b, plugin_host_static.NewController([]plugin_host.PluginHost{
		plugin_host_mock.NewHost(platformID),
	}))
}

// startSpaceRuntimeWatch runs WatchState on r. stop ends the watch and checks
// that it ended cleanly.
func startSpaceRuntimeWatch(
	t *testing.T,
	ctx context.Context,
	r *SpaceContentsResource,
) (*testWatchSpaceContentsStateStream, func()) {
	t.Helper()
	watchCtx, watchCancel := context.WithCancel(ctx)
	stream := newTestWatchSpaceContentsStateStream(watchCtx)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- r.WatchState(&s4wave_space.WatchSpaceContentsStateRequest{}, stream)
	}()
	return stream, func() {
		t.Helper()
		watchCancel()
		if err := <-watchErr; err != nil && !errors.Is(err, context.Canceled) {
			t.Fatalf("WatchState: %v", err)
		}
	}
}

// watchSpaceRuntimeState watches r until done accepts a snapshot.
func watchSpaceRuntimeState(
	t *testing.T,
	ctx context.Context,
	r *SpaceContentsResource,
	done func(*s4wave_space.SpaceContentsState) bool,
) {
	t.Helper()
	stream, stop := startSpaceRuntimeWatch(t, ctx, r)
	defer stop()
	for !done(recvSpaceRuntimeWatchState(t, stream)) {
	}
}

// recvSpaceRuntimeWatchState receives the next WatchState snapshot.
func recvSpaceRuntimeWatchState(
	t *testing.T,
	stream *testWatchSpaceContentsStateStream,
) *s4wave_space.SpaceContentsState {
	t.Helper()
	select {
	case state := <-stream.msgs:
		return state
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for Space contents state")
		return nil
	}
}
