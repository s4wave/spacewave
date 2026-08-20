package resource_space

import (
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/controller"
	controller_exec "github.com/aperturerobotics/controllerbus/controller/exec"
	controllerbus_core "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/echo"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/broadcast"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_entrypoint_controller "github.com/s4wave/spacewave/bldr/plugin/entrypoint/controller"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

const spaceRuntimeManifestID = "cold-plugin"

func TestSpaceRuntimeBusBridgesOnlyInfrastructure(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	parent, _, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	recorded := newSpaceRuntimeDirectiveRecorder()
	ref, err := parent.AddController(ctx, recorded, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ref()

	child, resolver, err := controllerbus_core.NewCoreBus(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	bridgeRef, err := child.AddController(ctx, bus_bridge.NewBusBridge(parent, spaceRuntimeBridgeFilter), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer bridgeRef()
	mirrorRef, err := child.AddController(ctx, newSpacePluginHostMirror(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer mirrorRef()
	_ = resolver

	allowed := []directive.Directive{
		world.NewLookupWorldEngine("engine"),
		world.NewLookupWorldOp("operation", "engine"),
		volume.NewLookupVolume("volume", ""),
		volume.NewBuildObjectStoreAPI("store", "volume"),
		plugin_host_root.NewLookupRoot([]string{"desktop/darwin/arm64"}),
	}
	for _, dir := range allowed {
		execSpaceRuntimeDirective(t, ctx, child, dir)
		recorded.wait(t, dir)
	}

	blocked := []directive.Directive{
		plugin_host.NewLookupPluginHost(nil),
		bldr_plugin.NewLoadPluginInstanced("plugin", "space-a"),
		bldr_manifest.NewFetchManifest("plugin", nil, nil, 0),
		bifrost_rpc.NewLookupRpcClient(bldr_plugin.SRPCPluginServiceID, "plugin"),
		bifrost_rpc.NewLookupRpcService(bldr_plugin.SRPCPluginHostServiceID, "plugin-host"),
	}
	for _, dir := range blocked {
		execSpaceRuntimeDirective(t, ctx, child, dir)
	}
	recorded.assertNoMore(t)
}

func TestSpaceRuntimeSchedulesApprovedPluginFromParentManifestSource(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	parentLoads := newSpaceRuntimeLoadPluginRecorder()
	parentLoadsRef, err := tb.Bus.AddController(ctx, parentLoads, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer parentLoadsRef()

	volumeRef, err := tb.Bus.AddController(ctx, &spaceRuntimeVolumeAliasController{volume: tb.Volume}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer volumeRef()
	hostRef := addSpaceRuntimePluginHost(t, ctx, tb.Bus, "test/platform")
	defer hostRef()
	manifestSource := newSpaceRuntimeManifestSourceController()
	sourceRef, err := tb.Bus.AddController(ctx, manifestSource, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sourceRef()

	resource := NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, "space-test", tb.EngineID)
	resource.StartController(&plugin_space.Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	})
	defer resource.Release()
	waitSpaceRuntimeStarted(t, resource)

	select {
	case <-manifestSource.started:
		t.Fatal("unapproved Space plugin fetched its parent manifest")
	default:
	}

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

	select {
	case <-manifestSource.started:
	case <-ctx.Done():
		t.Fatal("approved Space plugin did not fetch its parent manifest")
	}

	scheduler := resource.runtime.scheduler
	requested := false
	for _, status := range scheduler.GetPluginStatusCtr().GetValue().Plugins {
		if status.GetPluginId() == spaceRuntimeManifestID &&
			status.GetInstanceKey() == "space-test" &&
			status.GetState() == bldr_plugin.PluginState_PluginState_REQUESTED {
			requested = true
			break
		}
	}
	if !requested {
		t.Fatalf("plugin lifecycle did not reach requested: %#v", scheduler.GetPluginStatusCtr().GetValue())
	}
	select {
	case load := <-parentLoads.loads:
		t.Fatalf("empty HostPluginId leaked LoadPlugin to parent: %#v", load)
	default:
	}
}

func TestSpaceRuntimeRoutesPluginHostLoadToParentEntrypoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	volumeRef, err := tb.Bus.AddController(ctx, &spaceRuntimeVolumeAliasController{volume: tb.Volume}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer volumeRef()

	host := newSpaceRuntimeEntrypointHost()
	mux := srpc.NewMux()
	if err := bldr_plugin.SRPCRegisterPluginHost(mux, host); err != nil {
		t.Fatal(err)
	}
	entrypoint := plugin_entrypoint_controller.NewController(
		tb.Bus,
		tb.Logger,
		&bldr_plugin.PluginMeta{PluginId: "spacewave-core"},
		bldr_plugin.NewSRPCPluginHostClient(srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(mux)))),
	)
	entrypointRef, err := tb.Bus.AddController(ctx, entrypoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer entrypointRef()

	resource := NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, "space-test", tb.EngineID)
	resource.volumeID = tb.EngineVolumeID
	resource.storeID = tb.EngineObjectStoreID
	resource.StartController(&plugin_space.Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
		HostPluginId:  "spacewave-core",
	})
	defer resource.Release()
	waitSpaceRuntimeStarted(t, resource)

	if _, _, err := space_world_ops.SetSpaceSettings(
		ctx, tb.WorldState, peer.ID(""), "",
		&space_world.SpaceSettings{PluginIds: []string{spaceRuntimeManifestID}}, true, time.Now(),
	); err != nil {
		t.Fatal(err)
	}

	select {
	case req := <-host.requests:
		if req.GetPluginId() != spaceRuntimeManifestID || req.GetInstanceKey() != "space-test" {
			t.Fatalf("LoadPlugin request = %#v", req)
		}
	case <-ctx.Done():
		t.Fatal("parent plugin entrypoint did not receive LoadPlugin")
	}

	watchCtx, watchCancel := context.WithCancel(ctx)
	stream := newTestWatchSpaceContentsStateStream(watchCtx)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- resource.WatchState(&s4wave_space.WatchSpaceContentsStateRequest{}, stream)
	}()
	state := recvSpaceRuntimeWatchState(t, stream)
	if len(state.GetPlugins()) != 1 || !state.GetPlugins()[0].GetLoaded() ||
		state.GetPlugins()[0].GetState() != s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_LOADED {
		t.Fatalf("plugin lifecycle = %#v", state.GetPlugins())
	}
	watchCancel()
	if err := <-watchErr; err != nil && err != context.Canceled {
		t.Fatalf("WatchState: %v", err)
	}

	resource.Release()
	select {
	case <-host.released:
	case <-ctx.Done():
		t.Fatal("releasing Space did not cancel parent LoadPlugin stream")
	}
}

func TestSpaceRuntimeRestartsAfterParentHostPublication(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	volumeRef, err := tb.Bus.AddController(ctx, &spaceRuntimeVolumeAliasController{volume: tb.Volume}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer volumeRef()
	hosts := newSpaceRuntimeMutablePluginHostController()
	hostsRef, err := tb.Bus.AddController(ctx, hosts, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hostsRef()
	manifestSource := newSpaceRuntimeManifestSourceController()
	sourceRef, err := tb.Bus.AddController(ctx, manifestSource, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sourceRef()

	resource := NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, "space-test", tb.EngineID)
	resource.StartController(&plugin_space.Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	})
	defer resource.Release()
	waitSpaceRuntimeStarted(t, resource)
	firstRuntime := resource.runtime

	host := &spaceRuntimePluginHost{platformID: "test/platform"}
	hosts.SetHosts([]plugin_host.PluginHost{host})
	if err := resource.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return resource.runtime != nil && resource.runtime != firstRuntime && resource.startErr == nil, nil
	}); err != nil {
		t.Fatalf("Space runtime did not restart after parent host publication: %v", err)
	}
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
	select {
	case <-manifestSource.started:
	case <-ctx.Done():
		t.Fatal("restarted Space runtime did not fetch the approved parent manifest")
	}
}

func TestBindAttachedRpcServiceRebindsAfterSpaceRuntimeReplacement(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	volumeRef, err := tb.Bus.AddController(ctx, &spaceRuntimeVolumeAliasController{volume: tb.Volume}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer volumeRef()
	hosts := newSpaceRuntimeMutablePluginHostController()
	hostsRef, err := tb.Bus.AddController(ctx, hosts, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hostsRef()
	manifestSource := newSpaceRuntimeManifestSourceController()
	sourceRef, err := tb.Bus.AddController(ctx, manifestSource, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer sourceRef()

	resource := NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, "space-test", tb.EngineID)
	resource.StartController(&plugin_space.Config{
		SpaceId:       "space-test",
		VolumeId:      tb.EngineVolumeID,
		ObjectStoreId: tb.EngineObjectStoreID,
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	})
	defer resource.Release()
	waitSpaceRuntimeStarted(t, resource)
	firstRuntime := resource.runtime

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
	attachedMux := srpc.NewMux()
	if err := attachedMux.Register(echo.NewSRPCEchoerHandler(echo.NewEchoServer(nil), echo.SRPCEchoerServiceID)); err != nil {
		t.Fatal(err)
	}
	attachedID, err := attachedResources.AddResource(attachedMux, nil)
	if err != nil {
		t.Fatal(err)
	}
	bindCtx := resource_server.WithResourceClientContext(ctx, attachedResources)
	stream := newAttachedRpcServiceStream(bindCtx)
	stream.checkReady = func() error {
		return invokeAttachedEcho(ctx, resource, "first response")
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
	if err := invokeAttachedEcho(ctx, resource, "before replacement"); err != nil {
		t.Fatal(err)
	}

	// Replace the isolated runtime while the old bind continuation is paused.
	hosts.SetHosts([]plugin_host.PluginHost{&spaceRuntimePluginHost{platformID: "test/platform"}})
	if err := resource.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return resource.runtime != nil && resource.runtime != firstRuntime && resource.startErr == nil, nil
	}); err != nil {
		t.Fatalf("Space runtime did not restart: %v", err)
	}
	select {
	case <-stream.ready:
		t.Fatal("old runtime reported readiness after replacement")
	default:
	}

	close(continueBind)
	select {
	case <-stream.ready:
	case <-ctx.Done():
		t.Fatal("replacement attached route did not become ready")
	}
	if err := invokeAttachedEcho(ctx, resource, "after replacement"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-bindDone:
		t.Fatalf("bind stream ended after runtime replacement: %v", err)
	default:
	}

	if !attachedResources.ReleaseResource(attachedID) {
		t.Fatal("attached resource release failed")
	}
	if err := <-bindDone; err != nil {
		t.Fatalf("bind returned %v after attached resource release", err)
	}
	if err := assertAttachedEchoAbsent(ctx, resource); err != nil {
		t.Fatal(err)
	}
}

// invokeAttachedEcho calls the current Space runtime through its attached route.
func invokeAttachedEcho(ctx context.Context, resource *SpaceContentsResource, body string) error {
	runtime, err := currentSpaceRuntime(resource)
	if err != nil {
		return err
	}
	serviceID := "attached/" + echo.SRPCEchoerServiceID
	invoker := bifrost_rpc.NewInvoker(runtime.bus, "", false)
	client := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(invoker)))
	response, err := echo.NewSRPCEchoerClientWithServiceID(client, serviceID).Echo(ctx, &echo.EchoMsg{Body: body})
	if err != nil {
		return err
	}
	if response.GetBody() != body {
		return errors.New("attached echo response body differs")
	}
	return nil
}

func assertAttachedEchoAbsent(ctx context.Context, resource *SpaceContentsResource) error {
	runtime, err := currentSpaceRuntime(resource)
	if err != nil {
		return err
	}
	values, _, valuesRef, err := bifrost_rpc.ExLookupRpcService(
		ctx,
		runtime.bus,
		"attached/"+echo.SRPCEchoerServiceID,
		"",
		false,
		nil,
	)
	if valuesRef != nil {
		valuesRef.Release()
	}
	if err != nil {
		return err
	}
	if len(values) != 0 {
		return errors.New("released attachment remained callable")
	}
	return nil
}

func currentSpaceRuntime(resource *SpaceContentsResource) (*spaceRuntime, error) {
	var runtime *spaceRuntime
	resource.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		runtime = resource.runtime
	})
	if runtime == nil {
		return nil, errors.New("Space runtime is not running")
	}
	return runtime, nil
}

func TestSpaceContentsResourceProjectsPluginHostWatchChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	if _, _, err := space_world_ops.SetSpaceSettings(ctx, tb.WorldState, peer.ID(""), "", &space_world.SpaceSettings{PluginIds: []string{"test-plugin"}}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	firstRef := addSpaceRuntimePluginHost(t, ctx, tb.Bus, "desktop/test-a")
	defer firstRef()

	resource := NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, "space-test", tb.EngineID)
	resource.volumeID = tb.EngineVolumeID
	resource.StartController(&plugin_space.Config{
		SpaceId:       "space-test",
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	})
	defer resource.Release()
	waitSpaceRuntimeStarted(t, resource)
	firstRuntime := resource.runtime

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	stream := newTestWatchSpaceContentsStateStream(watchCtx)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- resource.WatchState(&s4wave_space.WatchSpaceContentsStateRequest{}, stream)
	}()
	<-stream.msgs

	secondRef := addSpaceRuntimePluginHost(t, ctx, tb.Bus, "desktop/test-b")
	defer secondRef()
	if err := resource.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return resource.runtime != nil && resource.runtime != firstRuntime && resource.startErr == nil, nil
	}); err != nil {
		t.Fatalf("Space runtime did not restart after daemon plugin host change: %v", err)
	}
	state := recvSpaceRuntimeWatchState(t, stream)
	if len(state.GetPlugins()) != 1 {
		t.Fatalf("plugins = %d, want 1", len(state.GetPlugins()))
	}
	if state.GetPlugins()[0].GetState() == s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED {
		t.Fatalf("plugin state = %#v", state.GetPlugins()[0])
	}
	watchCancel()
	if err := <-watchErr; err != nil && err != context.Canceled {
		t.Fatalf("WatchState: %v", err)
	}
}

func TestSpaceContentsResourceProjectsPluginHostWatchError(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	if _, _, err := space_world_ops.SetSpaceSettings(ctx, tb.WorldState, peer.ID(""), "", &space_world.SpaceSettings{PluginIds: []string{"test-plugin"}}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	firstRef := addSpaceRuntimePluginHost(t, ctx, tb.Bus, "desktop/test-a")
	defer firstRef()

	resource := NewSpaceContentsResource(tb.Logger, tb.Bus, tb.Engine, "space-test", tb.EngineID)
	resource.volumeID = tb.EngineVolumeID
	resource.StartController(&plugin_space.Config{
		SpaceId:       "space-test",
		EngineId:      tb.EngineID,
		SessionPeerId: tb.Volume.GetPeerID().String(),
	})
	defer resource.Release()
	waitSpaceRuntimeStarted(t, resource)

	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	stream := newTestWatchSpaceContentsStateStream(watchCtx)
	watchErr := make(chan error, 1)
	go func() {
		watchErr <- resource.WatchState(&s4wave_space.WatchSpaceContentsStateRequest{}, stream)
	}()
	<-stream.msgs

	errRef, err := tb.Bus.AddController(ctx, spaceRuntimePluginHostErrorController{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer errRef()
	state := recvSpaceRuntimeWatchState(t, stream)
	plugin := state.GetPlugins()[0]
	if plugin.GetState() != s4wave_space.SpacePluginLifecycleState_SpacePluginLifecycleState_FAILED || plugin.GetDetail() != "watch daemon plugin hosts: test plugin host watch error" {
		t.Fatalf("plugin state = %#v", plugin)
	}
	waitSpaceRuntimeReleased(t, resource)
	watchCancel()
	if err := <-watchErr; err != nil && err != context.Canceled {
		t.Fatalf("WatchState: %v", err)
	}
}

type spaceRuntimeDirectiveRecorder struct {
	mu   sync.Mutex
	dirs []directive.Directive
	ch   chan struct{}
}

func newSpaceRuntimeDirectiveRecorder() *spaceRuntimeDirectiveRecorder {
	return &spaceRuntimeDirectiveRecorder{ch: make(chan struct{}, 16)}
}

func (c *spaceRuntimeDirectiveRecorder) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/space-runtime-recorder", controller.MustParseVersion("0.0.1"), "records bridged directives")
}

func (c *spaceRuntimeDirectiveRecorder) Execute(context.Context) error { return nil }
func (c *spaceRuntimeDirectiveRecorder) Close() error                  { return nil }

func (c *spaceRuntimeDirectiveRecorder) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	c.mu.Lock()
	c.dirs = append(c.dirs, inst.GetDirective())
	c.mu.Unlock()
	c.ch <- struct{}{}
	return directive.R(directive.NewFuncResolver(func(_ context.Context, handler directive.ResolverHandler) error {
		handler.MarkIdle(true)
		return nil
	}), nil)
}

func (c *spaceRuntimeDirectiveRecorder) wait(t *testing.T, want directive.Directive) {
	t.Helper()
	select {
	case <-c.ch:
	case <-time.After(time.Second):
		t.Fatalf("parent did not receive %T", want)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	got := c.dirs[len(c.dirs)-1]
	if equivalent, ok := want.(directive.DirectiveWithEquiv); !ok || !equivalent.IsEquivalent(got) {
		t.Fatalf("parent directive = %T, want %T", got, want)
	}
}

func (c *spaceRuntimeDirectiveRecorder) assertNoMore(t *testing.T) {
	t.Helper()
	select {
	case <-c.ch:
		t.Fatal("blocked Space directive reached parent")
	case <-time.After(50 * time.Millisecond):
	}
}

func execSpaceRuntimeDirective(t *testing.T, ctx context.Context, b bus.Bus, dir directive.Directive) {
	t.Helper()
	_, ref, err := b.AddDirective(dir, bus.NewCallbackHandler(nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ref.Release)
}

func addSpaceRuntimePluginHost(t *testing.T, ctx context.Context, b bus.Bus, platformID string) func() {
	t.Helper()
	ref, err := b.AddController(ctx, &spaceRuntimePluginHostController{host: &spaceRuntimePluginHost{platformID: platformID}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

func waitSpaceRuntimeStarted(t *testing.T, r *SpaceContentsResource) {
	t.Helper()
	if err := r.bcast.Wait(t.Context(), func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return r.runtime != nil || r.startErr != nil, nil
	}); err != nil {
		t.Fatal(err)
	}
	if r.startErr != nil {
		t.Fatal(r.startErr)
	}
}

func waitSpaceRuntimeReleased(t *testing.T, r *SpaceContentsResource) {
	t.Helper()
	if err := r.bcast.Wait(t.Context(), func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return r.runtime == nil && r.ctrlRef == nil && r.startErr != nil, nil
	}); err != nil {
		t.Fatal(err)
	}
}

func recvSpaceRuntimeWatchState(t *testing.T, stream *testWatchSpaceContentsStateStream) *s4wave_space.SpaceContentsState {
	t.Helper()
	select {
	case state := <-stream.msgs:
		return state
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for terminal Space state")
		return nil
	}
}

type spaceRuntimeManifestSourceController struct {
	started chan struct{}
	once    sync.Once
}

func newSpaceRuntimeManifestSourceController() *spaceRuntimeManifestSourceController {
	return &spaceRuntimeManifestSourceController{started: make(chan struct{})}
}

func (c *spaceRuntimeManifestSourceController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/space-runtime-manifest-source", controller.MustParseVersion("0.0.1"), "test manifest source")
}

func (c *spaceRuntimeManifestSourceController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}
func (c *spaceRuntimeManifestSourceController) Close() error { return nil }
func (c *spaceRuntimeManifestSourceController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(bldr_manifest.FetchManifest)
	if !ok || dir.GetManifestId() != spaceRuntimeManifestID {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		c.once.Do(func() { close(c.started) })
		_, _ = handler.AddValue(&bldr_manifest.FetchManifestValue{})
		handler.MarkIdle(true)
		<-ctx.Done()
		return ctx.Err()
	}), nil)
}

type spaceRuntimeVolumeAliasController struct{ volume volume.Volume }

func (c *spaceRuntimeVolumeAliasController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/space-runtime-volume-alias", controller.MustParseVersion("0.0.1"), "test plugin host volume alias")
}
func (c *spaceRuntimeVolumeAliasController) Execute(context.Context) error { return nil }
func (c *spaceRuntimeVolumeAliasController) Close() error                  { return nil }
func (c *spaceRuntimeVolumeAliasController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	dir, ok := inst.GetDirective().(volume.LookupVolume)
	if !ok || dir.LookupVolumeID() != bldr_plugin.PluginVolumeID {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]volume.Volume{c.volume}), nil)
}

type spaceRuntimeMutablePluginHostController struct {
	bcast broadcast.Broadcast
	hosts []plugin_host.PluginHost
}

func newSpaceRuntimeMutablePluginHostController() *spaceRuntimeMutablePluginHostController {
	return &spaceRuntimeMutablePluginHostController{}
}

func (c *spaceRuntimeMutablePluginHostController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/space-runtime-mutable-plugin-host", controller.MustParseVersion("0.0.1"), "test mutable plugin host")
}
func (c *spaceRuntimeMutablePluginHostController) Execute(context.Context) error { return nil }
func (c *spaceRuntimeMutablePluginHostController) Close() error                  { return nil }
func (c *spaceRuntimeMutablePluginHostController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(plugin_host.LookupPluginHost); !ok {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(ctx context.Context, handler directive.ResolverHandler) error {
		for {
			var hosts []plugin_host.PluginHost
			var waitCh <-chan struct{}
			c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				hosts = slices.Clone(c.hosts)
				waitCh = getWaitCh()
			})
			_ = handler.ClearValues()
			for _, host := range hosts {
				_, _ = handler.AddValue(host)
			}
			handler.MarkIdle(true)
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-waitCh:
			}
		}
	}), nil)
}

func (c *spaceRuntimeMutablePluginHostController) SetHosts(hosts []plugin_host.PluginHost) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.hosts = slices.Clone(hosts)
		broadcast()
	})
}

type spaceRuntimeLoadPluginRecorder struct {
	loads chan bldr_plugin.LoadPlugin
}

func newSpaceRuntimeLoadPluginRecorder() *spaceRuntimeLoadPluginRecorder {
	return &spaceRuntimeLoadPluginRecorder{loads: make(chan bldr_plugin.LoadPlugin, 1)}
}

// GetControllerInfo returns the test controller metadata.
func (c *spaceRuntimeLoadPluginRecorder) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/space-runtime-load-recorder", controller.MustParseVersion("0.0.1"), "records parent plugin loads")
}

// Execute keeps the test controller available on its bus.
func (c *spaceRuntimeLoadPluginRecorder) Execute(context.Context) error { return nil }

// Close releases no resources for this test controller.
func (c *spaceRuntimeLoadPluginRecorder) Close() error { return nil }

// HandleDirective records parent plugin load directives.
func (c *spaceRuntimeLoadPluginRecorder) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	load, ok := inst.GetDirective().(bldr_plugin.LoadPlugin)
	if !ok {
		return nil, nil
	}
	c.loads <- load
	return directive.R(directive.NewFuncResolver(func(_ context.Context, handler directive.ResolverHandler) error {
		handler.MarkIdle(true)
		return nil
	}), nil)
}

type spaceRuntimeEntrypointHost struct {
	requests chan *bldr_plugin.LoadPluginRequest
	released chan struct{}
	once     sync.Once
}

func newSpaceRuntimeEntrypointHost() *spaceRuntimeEntrypointHost {
	return &spaceRuntimeEntrypointHost{
		requests: make(chan *bldr_plugin.LoadPluginRequest, 1),
		released: make(chan struct{}),
	}
}

// GetPluginInfo returns empty test plugin metadata.
func (h *spaceRuntimeEntrypointHost) GetPluginInfo(context.Context, *bldr_plugin.GetPluginInfoRequest) (*bldr_plugin.GetPluginInfoResponse, error) {
	return &bldr_plugin.GetPluginInfoResponse{}, nil
}

// ExecController rejects the unused controller execution path.
func (h *spaceRuntimeEntrypointHost) ExecController(*controller_exec.ExecControllerRequest, bldr_plugin.SRPCPluginHost_ExecControllerStream) error {
	return errors.New("ExecController is not used by this test")
}

// LoadPlugin records one remote plugin load and remains active for its stream lifetime.
func (h *spaceRuntimeEntrypointHost) LoadPlugin(req *bldr_plugin.LoadPluginRequest, strm bldr_plugin.SRPCPluginHost_LoadPluginStream) error {
	h.requests <- req.CloneVT()
	if err := strm.Send(&bldr_plugin.LoadPluginResponse{PluginStatus: &bldr_plugin.PluginStatus{Running: true}}); err != nil {
		return err
	}
	<-strm.Context().Done()
	h.once.Do(func() { close(h.released) })
	return strm.Context().Err()
}

// PluginRpc rejects the unused plugin RPC path.
func (h *spaceRuntimeEntrypointHost) PluginRpc(bldr_plugin.SRPCPluginHost_PluginRpcStream) error {
	return errors.New("PluginRpc is not used by this test")
}

// PluginFsRpc rejects the unused plugin filesystem RPC path.
func (h *spaceRuntimeEntrypointHost) PluginFsRpc(bldr_plugin.SRPCPluginHost_PluginFsRpcStream) error {
	return errors.New("PluginFsRpc is not used by this test")
}

var _ bldr_plugin.SRPCPluginHostServer = (*spaceRuntimeEntrypointHost)(nil)

type spaceRuntimePluginHostController struct{ host plugin_host.PluginHost }

func (c *spaceRuntimePluginHostController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/space-runtime-plugin-host", controller.MustParseVersion("0.0.1"), "test plugin host")
}
func (c *spaceRuntimePluginHostController) Execute(context.Context) error { return nil }
func (c *spaceRuntimePluginHostController) Close() error                  { return nil }
func (c *spaceRuntimePluginHostController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(plugin_host.LookupPluginHost); !ok {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]plugin_host.PluginHost{c.host}), nil)
}

type spaceRuntimePluginHost struct{ platformID string }

func (h *spaceRuntimePluginHost) GetPlatformId() string                         { return h.platformID }
func (h *spaceRuntimePluginHost) Execute(context.Context) error                 { return nil }
func (h *spaceRuntimePluginHost) ListPlugins(context.Context) ([]string, error) { return nil, nil }
func (h *spaceRuntimePluginHost) ExecutePlugin(context.Context, string, string, string, *unixfs.FSHandle, *unixfs.FSHandle, srpc.Mux, plugin_host.PluginRpcInitCb) error {
	return nil
}
func (h *spaceRuntimePluginHost) DeletePlugin(context.Context, string) error { return nil }

type spaceRuntimePluginHostErrorController struct{}

func (spaceRuntimePluginHostErrorController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/space-runtime-plugin-host-error", controller.MustParseVersion("0.0.1"), "test plugin host watch error")
}
func (spaceRuntimePluginHostErrorController) Execute(context.Context) error { return nil }
func (spaceRuntimePluginHostErrorController) Close() error                  { return nil }
func (spaceRuntimePluginHostErrorController) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(plugin_host.LookupPluginHost); !ok {
		return nil, nil
	}
	return directive.R(directive.NewFuncResolver(func(context.Context, directive.ResolverHandler) error {
		return errors.New("test plugin host watch error")
	}), nil)
}
