package resource_space

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	plugin_host_mock "github.com/s4wave/spacewave/bldr/plugin/host/mock"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	"github.com/s4wave/spacewave/testbed"
)

// continuityPlatform is the platform of the installed plugin artifact.
const continuityPlatform = "desktop/linux/amd64"

// continuityQuiet bounds the wait that proves a host change started no work.
const continuityQuiet = 100 * time.Millisecond

// continuityHost is a plugin host that runs each plugin until its context ends
// and reports the start and stop of every execution as "name:start" and
// "name:stop".
type continuityHost struct {
	*plugin_host_mock.Host
	// name identifies the host in events.
	name string
	// events receives the execution events of every host of the fixture.
	events chan<- string
}

// ExecutePlugin runs the plugin until ctx ends.
func (h *continuityHost) ExecutePlugin(
	ctx context.Context,
	_, _, _, _, _ string,
	_, _ *unixfs.FSHandle,
	_ srpc.Mux,
	_ plugin_host.PluginRpcInitCb,
) error {
	h.events <- h.name + ":start"
	<-ctx.Done()
	h.events <- h.name + ":stop"
	return context.Canceled
}

// _ is a type assertion
var _ plugin_host.PluginHost = (*continuityHost)(nil)

// hostLookupObserver counts the open LookupPluginHost directives that reach the
// daemon bus, each of which holds the hosts it resolved.
type hostLookupObserver struct {
	// open is the number of LookupPluginHost directives not yet disposed.
	open *ccontainer.CContainer[int]
}

// newHostLookupObserver constructs an observer with no open lookups.
func newHostLookupObserver() *hostLookupObserver {
	return &hostLookupObserver{open: ccontainer.NewCContainer(0)}
}

// GetControllerInfo returns information about the controller.
func (o *hostLookupObserver) GetControllerInfo() *controller.Info {
	return controller.NewInfo("test/host-lookup-observer", controller.MustParseVersion("0.0.1"), "counts host lookups")
}

// Execute executes the controller.
func (o *hostLookupObserver) Execute(context.Context) error {
	return nil
}

// HandleDirective counts the lookup until its instance is disposed.
func (o *hostLookupObserver) HandleDirective(_ context.Context, inst directive.Instance) ([]directive.Resolver, error) {
	if _, ok := inst.GetDirective().(plugin_host.LookupPluginHost); !ok {
		return nil, nil
	}
	o.open.SwapValue(func(n int) int { return n + 1 })
	inst.AddDisposeCallback(func() { o.open.SwapValue(func(n int) int { return n - 1 }) })
	return nil, nil
}

// Close releases any resources used by the controller.
func (o *hostLookupObserver) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*hostLookupObserver)(nil)

// continuityFixture is a daemon bus with an installed plugin and the hosts that
// can run it.
type continuityFixture struct {
	ctx context.Context
	tb  *testbed.Testbed
	// events receives the execution events of the fixture's hosts.
	events chan string
	// lookups counts the open host lookups on the daemon bus.
	lookups *hostLookupObserver
}

// newContinuityFixture starts a daemon bus without hosts or mounts.
func newContinuityFixture(t *testing.T) *continuityFixture {
	// Build the fixture around a fresh daemon bus.
	t.Helper()
	ctx, tb := newSpaceRuntimeTestbed(t, 30*time.Second)
	f := &continuityFixture{
		ctx:     ctx,
		tb:      tb,
		events:  make(chan string, 64),
		lookups: newHostLookupObserver(),
	}

	// Count the host lookups that open on the bus.
	addSpaceRuntimeController(t, tb.Bus, f.lookups)
	return f
}

// addHost publishes a plugin host until the returned release runs or the test
// ends.
func (f *continuityFixture) addHost(t *testing.T, name, platform string) (*continuityHost, func()) {
	// Serve the host on the daemon bus.
	t.Helper()
	host := &continuityHost{Host: plugin_host_mock.NewHost(platform), name: name, events: f.events}
	release, err := f.tb.Bus.AddController(t.Context(), plugin_host_controller.NewController(
		f.tb.Logger,
		f.tb.Bus,
		controller.NewInfo("test/continuity-host/"+name, controller.MustParseVersion("0.0.1"), ""),
		host,
	), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Allow the test to release the host early.
	release = sync.OnceFunc(release)
	t.Cleanup(release)
	return host, release
}

// mount mounts the Space, which shares the runtime of every other mount.
func (f *continuityFixture) mount(t *testing.T) *SpaceContentsResource {
	// Mount the Space against the daemon bus.
	t.Helper()
	mount := newTestSpaceContentsResource(t, f.tb.Logger, f.tb.Bus, f.tb.Engine, newSpaceRuntimeConfig(f.tb))
	mount.volumeID = f.tb.EngineVolumeID
	mount.storeID = f.tb.EngineObjectStoreID
	return mount
}

// install stores an executable plugin artifact in the Space and installs it.
func (f *continuityFixture) install(t *testing.T) {
	// Describe an artifact with an entrypoint for the host platform.
	t.Helper()
	distFS := memfs.New()
	if err := billy_util.WriteFile(distFS, "entrypoint.js", []byte("export {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := bldr_manifest.NewManifest(
		bldr_manifest.NewManifestMeta(spaceRuntimeManifestID, bldr_manifest.BuildType_DEV, continuityPlatform, 1),
		"entrypoint.js",
	)

	// Stage the World state that retains the artifact.
	stage, err := f.tb.Engine.StageWorldState(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stage.Release)

	// Write the artifact blocks and record the object reference.
	var objectRef *bucket.ObjectRef
	if err := stage.AccessWorldState(f.ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		// Write the manifest and its dist files.
		transaction, blocks := cursor.BuildTransactionAtRef(nil, nil)
		if err := bldr_manifest.CreateManifestWithBilly(f.ctx, blocks, manifest, distFS, nil, timestamppb.Now()); err != nil {
			return err
		}
		rootRef, _, err := transaction.Write(f.ctx, true)
		if err != nil {
			return err
		}

		// Point the object reference at the written root.
		objectRef = cursor.GetRef().CloneVT()
		objectRef.RootRef = rootRef
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Register the artifact in the World.
	key := bldr_manifest.NewManifestArtifactKey(objectRef)
	if _, _, err := manifest_world.SetManifest(f.ctx, f.tb.WorldState, "", key, objectRef); err != nil {
		t.Fatal(err)
	}

	// Install the stored artifact for the Space.
	if _, _, err := space_world_ops.SetSpaceSettings(f.ctx, f.tb.WorldState, "", "", &space_world.SpaceSettings{
		PluginIds: []string{spaceRuntimeManifestID},
		PluginInstallations: map[string]*space_world.SpacePluginInstallation{
			spaceRuntimeManifestID: {ManifestKeys: []string{key}},
		},
	}, true, time.Now()); err != nil {
		t.Fatal(err)
	}
}

// next requires the next execution event to be want.
func (f *continuityFixture) next(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-f.events:
		if got != want {
			t.Fatalf("execution event %q, expected %q", got, want)
		}
	case <-f.ctx.Done():
		t.Fatalf("no execution event %q: %v", want, f.ctx.Err())
	}
}

// quiet requires that no execution event arrives for a short while. Execution
// events are the work a host change must not start.
func (f *continuityFixture) quiet(t *testing.T, step string) {
	t.Helper()
	select {
	case got := <-f.events:
		t.Fatalf("%s: unexpected execution event %q", step, got)
	case <-time.After(continuityQuiet):
	}
}

// awaitHosts waits until the scheduler's host state matches.
func (f *continuityFixture) awaitHosts(
	t *testing.T,
	scheduler *plugin_host_scheduler.Controller,
	match func([]plugin_host.PluginHost, error) bool,
) {
	t.Helper()
	for {
		hosts, wait, err := scheduler.GetHostState()
		if match(hosts, err) {
			return
		}
		if err := wait(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
}

// awaitLookups waits until the open host lookup count matches.
func (f *continuityFixture) awaitLookups(t *testing.T, match func(open int) bool) {
	t.Helper()
	_, err := f.lookups.open.WaitValueWithValidator(f.ctx, func(n int) (bool, error) { return match(n), nil }, nil)
	if err != nil {
		t.Fatalf("host lookups did not reach the expected count: %v", err)
	}
}

// hostsInclude returns a host state matcher for the live hosts and no error.
func hostsInclude(included []plugin_host.PluginHost, excluded ...plugin_host.PluginHost) func([]plugin_host.PluginHost, error) bool {
	return func(hosts []plugin_host.PluginHost, err error) bool {
		return err == nil &&
			!slices.ContainsFunc(included, func(h plugin_host.PluginHost) bool { return !slices.Contains(hosts, h) }) &&
			!slices.ContainsFunc(excluded, func(h plugin_host.PluginHost) bool { return slices.Contains(hosts, h) })
	}
}

// TestSpaceRuntimeHostChangesReplaceOnlyAffectedExecution proves the running
// plugin survives host changes that do not affect the host it runs on, moves to
// the actual remaining host when its own host leaves, and reports resolver
// errors without stopping.
func TestSpaceRuntimeHostChangesReplaceOnlyAffectedExecution(t *testing.T) {
	// Run the installed plugin on the first host of its platform, beside an
	// unused host of another platform.
	f := newContinuityFixture(t)
	web, releaseWeb := f.addHost(t, "web", "web/js/wasm")
	a1, releaseA1 := f.addHost(t, "a1", continuityPlatform)

	// Two mounts share one runtime.
	mount := f.mount(t)
	if f.mount(t).runtime != mount.runtime {
		t.Fatal("two mounts did not retain one runtime")
	}
	gen := waitSpaceRuntimeGeneration(t, mount.runtime, nil)
	scheduler := gen.GetScheduler()

	// Install the plugin and wait for it to run.
	f.install(t)
	f.next(t, "a1:start")

	// stable applies a host change and requires it to leave the runtime, the
	// mounts, and the execution untouched.
	stable := func(step string, match func([]plugin_host.PluginHost, error) bool) {
		t.Helper()
		f.awaitHosts(t, scheduler, match)
		f.quiet(t, step)
		if current, _, err := mount.runtime.GetGeneration(); current != gen || err != nil {
			t.Fatalf("%s: replaced the Space runtime", step)
		}
	}

	// A second host of the platform arrives, then the unused host leaves. The
	// daemon bus lists the newer host before the older one after the removal.
	a2, releaseA2 := f.addHost(t, "a2", continuityPlatform)
	stable("same-platform arrival", hostsInclude([]plugin_host.PluginHost{a1, a2}))
	releaseWeb()
	stable("unused host removal and snapshot reordering", hostsInclude([]plugin_host.PluginHost{a1, a2}, web))

	// Another unused host arrives.
	js, _ := f.addHost(t, "js", "js")
	stable("unused host arrival", hostsInclude([]plugin_host.PluginHost{a1, a2, js}))

	// The selected host leaves, so the execution moves to the remaining host.
	releaseA1()
	f.next(t, "a1:stop")
	f.next(t, "a2:start")

	// A replacement of the same platform arrives and the running host leaves.
	a3, _ := f.addHost(t, "a3", continuityPlatform)
	stable("replacement arrival", hostsInclude([]plugin_host.PluginHost{a2, a3}, a1))
	releaseA2()
	f.next(t, "a2:stop")
	f.next(t, "a3:start")

	// A resolver error is reported with the live hosts and starts no work.
	injected := errors.New("host resolver failed")
	releaseError, err := f.tb.Bus.AddController(t.Context(), plugin_host_mock.NewLookupErrorController(injected), nil)
	if err != nil {
		t.Fatal(err)
	}
	f.awaitHosts(t, scheduler, func(hosts []plugin_host.PluginHost, err error) bool {
		return errors.Is(err, injected) && slices.Contains(hosts, plugin_host.PluginHost(a3))
	})
	f.quiet(t, "resolver error")
	releaseError()
	stable("resolver recovery", hostsInclude([]plugin_host.PluginHost{a3}, a1, a2))
}

// TestSpaceRuntimeReleaseKeepsSharedRuntime proves releasing one mount retains
// the runtime, its execution, and the other mount's reservation, and releasing
// the last mount closes the host lookups and stops the plugin.
func TestSpaceRuntimeReleaseKeepsSharedRuntime(t *testing.T) {
	// Run the installed plugin through two mounts of the Space.
	f := newContinuityFixture(t)
	f.addHost(t, "a1", continuityPlatform)
	first := f.mount(t)
	second := f.mount(t)
	gen := waitSpaceRuntimeGeneration(t, first.runtime, nil)
	f.install(t)
	f.next(t, "a1:start")
	f.awaitLookups(t, func(open int) bool { return open > 0 })

	// Attach an echo service the mounts can bind.
	attachedResources := newSpaceRecordingResourceClient(f.ctx)
	attachedID := addAttachedEchoResource(t, attachedResources)
	bindCtx := resource_server.WithResourceClientContext(f.ctx, attachedResources)

	// Bind a service prefix through the first mount.
	stream := newAttachedRpcServiceStream(bindCtx)
	bindDone := make(chan error, 1)
	go func() {
		bindDone <- first.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
			AttachedResourceId: attachedID,
			ServiceIdPrefix:    "attached/",
		}, stream)
	}()
	<-stream.ready

	// Releasing the first mount frees its prefix and keeps everything running.
	first.Release()
	if err := <-bindDone; !errors.Is(err, errSpaceContentsReleased) {
		t.Fatalf("bind on released mount returned %v", err)
	}
	f.quiet(t, "release of one mount")

	// The runtime survives for the remaining mount.
	if current, _, err := second.runtime.GetGeneration(); current != gen || err != nil {
		t.Fatal("releasing one mount replaced the runtime")
	}
	select {
	case <-gen.Done():
		t.Fatal("releasing one mount stopped the runtime")
	default:
	}

	// The remaining mount binds the freed prefix.
	rebind := newAttachedRpcServiceStream(bindCtx)
	rebindDone := make(chan error, 1)
	go func() {
		rebindDone <- second.BindAttachedRpcService(&s4wave_space.BindAttachedRpcServiceRequest{
			AttachedResourceId: attachedID,
			ServiceIdPrefix:    "attached/",
		}, rebind)
	}()
	<-rebind.ready

	// Releasing the last mount stops the plugin and closes the host lookups.
	second.Release()
	if err := <-rebindDone; !errors.Is(err, errSpaceContentsReleased) {
		t.Fatalf("bind on the last released mount returned %v", err)
	}
	select {
	case <-gen.Done():
	case <-f.ctx.Done():
		t.Fatal("releasing the last mount did not stop the runtime")
	}
	f.next(t, "a1:stop")
	f.awaitLookups(t, func(open int) bool { return open == 0 })

	// A new mount starts a fresh runtime that runs the plugin again.
	third := f.mount(t)
	if third.runtime == first.runtime {
		t.Fatal("a released runtime was reused")
	}
	f.next(t, "a1:start")
}

// TestSpaceRuntimeRecoversExecutionAfterCompositionFailure proves a composition
// failure still replaces the generation: the old execution and host lookups
// close, and the plugin runs again on the same host.
func TestSpaceRuntimeRecoversExecutionAfterCompositionFailure(t *testing.T) {
	// Run the installed plugin and record the lookups of one generation.
	f := newContinuityFixture(t)
	f.addHost(t, "a1", continuityPlatform)
	mount := f.mount(t)
	first := waitSpaceRuntimeGeneration(t, mount.runtime, nil)
	f.install(t)
	f.next(t, "a1:start")
	open := f.lookups.open.GetValue()

	// Remove the scheduler from the generation's bus.
	first.GetBus().RemoveController(first.GetScheduler())
	waitSpaceRuntimeGeneration(t, mount.runtime, first)

	// The old execution stopped, the new generation runs the plugin, and the
	// old generation's lookups are gone.
	f.next(t, "a1:stop")
	f.next(t, "a1:start")
	f.awaitLookups(t, func(n int) bool { return n == open })
}
