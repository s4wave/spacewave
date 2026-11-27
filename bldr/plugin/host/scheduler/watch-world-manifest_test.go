package plugin_host_scheduler

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestWatchWorldManifestWakesOnLinkOfStoredManifest checks the running World
// watch selects a manifest whose object was already stored when a later store
// only links it to the plugin host. That store leaves the host's revision in
// place, so the watch must wake on the edge it read. The held case is the
// first selection after a relaunch: FetchManifest went idle and announced the
// manifest before its store linked it.
func TestWatchWorldManifestWakesOnLinkOfStoredManifest(t *testing.T) {
	for _, held := range []bool{false, true} {
		name := "selected"
		if held {
			name = "awaiting-fetch"
		}
		t.Run(name, func(t *testing.T) {
			testWatchWorldManifestWakesOnLinkOfStoredManifest(t, held)
		})
	}
}

func testWatchWorldManifestWakesOnLinkOfStoredManifest(t *testing.T, held bool) {
	// Bound the test and open a World testbed with a bus engine.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	ws := world.NewEngineWorldState(tb.Engine, true)

	// Create the plugin host and store a manifest object it does not link yet.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err)
	}
	var manifestRef *bldr_manifest.ManifestRef
	var manifestKey string
	if err := world.ExecTransaction(ctx, tb.Engine, true, func(ctx context.Context, tx world.WorldState) error {
		manifestRef, manifestKey = storeTestWorldManifest(t, ctx, tx, "spacewave-core", "desktop/darwin/arm64", 1)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Run the World manifest watch for the plugin.
	le := tb.Logger
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	hostSet := &pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host}}
	pi := &pluginInstance{
		c: &Controller{
			bus:    tb.Bus,
			conf:   &Config{EngineId: tb.EngineID},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	if held {
		// FetchManifest went idle having announced the manifest.
		pi.c.conf.AwaitFetchManifest = true
		pi.c.conf.WatchFetchManifest = true
		pi.announcedManifests.Store(&[]string{manifestKey})
	}
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go func() {
		_ = pi.execWatchWorldManifest(watchCtx, hostSet)
	}()

	// Wait for the first selection, which finds no linked manifest. A held
	// selection records nothing, so give it time to reach its wait.
	if held {
		time.Sleep(200 * time.Millisecond)
	}
	for !held && pi.manifestSelectionFingerprint.Load() == nil {
		if ctx.Err() != nil {
			t.Fatal("watch never completed its first selection")
		}
		time.Sleep(5 * time.Millisecond)
	}
	if pi.executePluginRoutine.GetState() != nil {
		t.Fatal("expected no selection before the manifest is linked")
	}

	// Store the fetched manifest. The manifest object already holds it, so the
	// store writes only the link from the plugin host.
	manifestRev := func() uint64 {
		obj, err := world.MustGetObject(ctx, ws, manifestKey)
		if err != nil {
			t.Fatal(err)
		}
		defer world.ReleaseObjectState(obj)
		_, rev, err := obj.GetRootRef(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return rev
	}
	before := manifestRev()
	if err := bldr_manifest_world.ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{objKey}, manifestRef); err != nil {
		t.Fatal(err)
	}
	if after := manifestRev(); after != before {
		t.Fatalf("manifest object revision moved from %d to %d; the case needs a link-only store", before, after)
	}

	// The watch must select the newly linked manifest.
	deadline := time.Now().Add(3 * time.Second)
	for {
		state := pi.executePluginRoutine.GetState()
		if state != nil && state.manifestSnapshot.GetManifestRef().EqualVT(manifestRef.GetManifestRef()) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("watch did not select the manifest linked to the plugin host")
		}
		time.Sleep(10 * time.Millisecond)
	}
}
func TestWatchWorldManifestUsesStartupManifestRefsAndSkipsBadCandidate(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest reference.
	goodRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/darwin/arm64", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, goodRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest reference.
	badRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/darwin/arm64", 9)
	badRef.GetManifestRef().RootRef.Hash.Hash[0] ^= 0xff
	const badRefKey = "plugin-host/ref/missing"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, badRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	pi := &pluginInstance{
		c: &Controller{
			bus:    tb.Bus,
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state from good startup manifest ref")
	}
	if execState.pluginHost != host {
		t.Fatal("expected execute state to use matching plugin host")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(goodRef.GetManifestRef()) {
		t.Fatal("expected skipped bad ref not to clear the good execute candidate")
	}
}

func TestWatchWorldManifestFiltersWebPlatformForUnlistedPlugin(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a manifest in the test World.
	webRef, webRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "web/js/wasm", 99)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, webRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}
	jsRef, jsRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "js", 1)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, jsRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}
	_ = webRef

	// Create the plugin host fixture.
	webHost := &testPluginHost{id: "web/js/wasm"}
	jsHost := &testPluginHost{id: "js"}
	pi := &pluginInstance{
		c: &Controller{
			bus:    tb.Bus,
			conf:   webPlatformAllowlistConfig("spacewave-v86"),
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{jsHost, webHost},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != jsHost {
		t.Fatal("expected unlisted startup plugin to use js fallback")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(jsRef.GetManifestRef()) {
		t.Fatal("expected unlisted startup plugin not to select web/js/wasm manifest")
	}
}

func TestWatchWorldManifestExecutesBootstrapManifestAndRecordsUnreadableRetainedRef(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a manifest in the test World.
	bootstrapRef, bootstrapRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 7)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, bootstrapRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Retain the manifest reference.
	retainedRef := newTestStoredManifestRef(t, ctx, tb, "other-plugin", "desktop/darwin/arm64", 9)
	const retainedRefKey = "plugin-host/ref/unreadable-retained"
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)
	corruptTestWorldObjectRoot(t, ctx, ws, retainedRefKey)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, retainedRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		bus:    tb.Bus,
		conf:   &Config{},
		objKey: objKey,
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state from readable bootstrap manifest")
	}
	if execState.pluginHost != host {
		t.Fatal("expected execute state to use matching plugin host")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(bootstrapRef.GetManifestRef()) {
		t.Fatal("expected unreadable retained ref not to clear the bootstrap execute candidate")
	}

	// Check the plugin status.
	status := ctrl.PluginStatus()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected one plugin status, got %d", len(status.Plugins))
	}
	lastError := status.Plugins[0].GetLastErrorMessage()
	if !strings.Contains(lastError, "startup manifest refs: 1 skipped startup manifest ref(s)") {
		t.Fatalf("unexpected retained-ref diagnostic: %q", lastError)
	}
	if !strings.Contains(lastError, retainedRefKey) {
		t.Fatalf("retained-ref diagnostic %q does not mention ref key %q", lastError, retainedRefKey)
	}
}

func TestWatchWorldManifestExecutesReadableLauncherWithUnavailableRetainedReleaseCdnCandidate(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "spacewave/launcher"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store the launcher manifest reference.
	launcherRef, launcherRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-launcher", "desktop/darwin/arm64", 12)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, launcherRefKey, "spacewave-launcher")); err != nil {
		t.Fatal(err.Error())
	}

	// Retain the manifest reference.
	retainedRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-launcher", "desktop/darwin/arm64", 13)
	retainedRef.GetManifestRef().BucketId = "spacewave-cdn-release-retained"
	const retainedRefKey = "release/manifests/spacewave-launcher/desktop/darwin/arm64/cdn-retained"
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)
	retainedEdge := bldr_manifest_world.NewManifestQuad(objKey, retainedRefKey, "")
	if err := ws.SetGraphQuad(ctx, retainedEdge); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		bus:    tb.Bus,
		conf:   &Config{},
		objKey: objKey,
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-launcher",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected launcher manifest store object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state from readable launcher candidate")
	}
	if execState.pluginHost != host {
		t.Fatal("expected execute state to use matching plugin host")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected unavailable retained release/CDN candidate not to replace the readable launcher candidate")
	}

	// Check the plugin status.
	status := ctrl.PluginStatus()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected one plugin status, got %d", len(status.Plugins))
	}
	lastError := status.Plugins[0].GetLastErrorMessage()
	if !strings.Contains(lastError, "startup manifest refs: 1 skipped startup manifest ref(s)") {
		t.Fatalf("unexpected retained release/CDN diagnostic: %q", lastError)
	}
	if !strings.Contains(lastError, retainedRefKey) {
		t.Fatalf("retained release/CDN diagnostic %q does not mention ref key %q", lastError, retainedRefKey)
	}
	if !strings.Contains(lastError, "bucket=spacewave-cdn-release-retained") {
		t.Fatalf("retained release/CDN diagnostic %q does not mention missing CDN bucket", lastError)
	}

	// Assert the expected state.
	if err := ws.DeleteGraphQuad(ctx, retainedEdge); err != nil {
		t.Fatal(err.Error())
	}
	deleted, err := ws.DeleteObject(ctx, retainedRefKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected retained release ref object to be deleted")
	}

	// Process the manifest World state.
	wait, _, err = pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// The watch loop must wait for changes after pruning.
	if !wait {
		t.Fatal("expected watch loop to wait for changes after pruning")
	}

	// Check the execute plugin routine state.
	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected readable launcher candidate to remain selected after pruning")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected pruned scan to keep the readable launcher candidate")
	}
	status = ctrl.PluginStatus()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected existing plugin status to remain, got %d", len(status.Plugins))
	}
	if lastError = status.Plugins[0].GetLastErrorMessage(); lastError != "" {
		t.Fatalf("expected startup manifest skip status to clear after pruning, got %q", lastError)
	}
}

func TestWatchWorldManifestIgnoresWrongPlatformRetainedRefAndSelectsCurrent(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a manifest in the test World.
	currentRef, currentRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 7)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, currentRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest reference.
	ignoredRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/linux/amd64", 99)
	const ignoredRefKey = "plugin-host/ref/wrong-platform"
	storeTestManifestRefObject(t, ctx, ws, ignoredRefKey, ignoredRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, ignoredRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		bus:    tb.Bus,
		conf:   &Config{},
		objKey: objKey,
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state from current platform candidate")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(currentRef.GetManifestRef()) {
		t.Fatal("expected wrong-platform retained ref to be ignored during execute selection")
	}
	if downloadState := pi.downloadManifestRoutine.GetState(); downloadState != nil &&
		downloadState.GetManifestRef().EqualVT(ignoredRef.GetManifestRef()) {
		t.Fatal("expected wrong-platform retained ref not to be scheduled for download")
	}

	// Check the plugin status.
	status := ctrl.PluginStatus()
	if len(status.Plugins) != 0 {
		t.Fatalf("expected ignored retained ref not to surface as a skip error, got %+v", status.Plugins)
	}
	{
		objectState, ok, err := ws.GetObject(ctx, ignoredRefKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		} else if !ok {
			t.Fatal("expected ignored retained ref to remain in the graph")
		}
	}
}

func TestWatchWorldManifestQuarantinesWrongManifestIDRetainedRef(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a manifest in the test World.
	currentRef, currentRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 7)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, currentRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest reference.
	quarantinedRef := newTestStoredManifestRef(t, ctx, tb, "other-plugin", "desktop/darwin/arm64", 99)
	const quarantinedRefKey = "plugin-host/ref/wrong-manifest-id"
	storeTestManifestRefObject(t, ctx, ws, quarantinedRefKey, quarantinedRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, quarantinedRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		bus:    tb.Bus,
		conf:   &Config{},
		objKey: objKey,
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state from compatible current candidate")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(currentRef.GetManifestRef()) {
		t.Fatal("expected quarantined retained ref not to replace current execute candidate")
	}
	if downloadState := pi.downloadManifestRoutine.GetState(); downloadState != nil &&
		downloadState.GetManifestRef().EqualVT(quarantinedRef.GetManifestRef()) {
		t.Fatal("expected quarantined retained ref not to be scheduled for download")
	}

	// Check the plugin status.
	status := ctrl.PluginStatus()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected one plugin status, got %d", len(status.Plugins))
	}
	lastError := status.Plugins[0].GetLastErrorMessage()
	for _, want := range []string{
		"startup manifest refs: 1 skipped startup manifest ref(s)",
		quarantinedRefKey,
		"quarantined",
		"manifest-id-mismatch:other-plugin",
	} {
		if !strings.Contains(lastError, want) {
			t.Fatalf("quarantine diagnostic %q does not contain %q", lastError, want)
		}
	}
	{
		objectState, ok, err := ws.GetObject(ctx, quarantinedRefKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		} else if !ok {
			t.Fatal("expected quarantined retained ref to remain in the graph")
		}
	}
}

func TestWatchWorldManifestClearsSkippedRefStatusAfterBucketFix(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "spacewave/launcher"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store the launcher manifest reference.
	launcherRef, launcherRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-launcher", "desktop/darwin/arm64", 12)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, launcherRefKey, "spacewave-launcher")); err != nil {
		t.Fatal(err.Error())
	}

	// Retain the manifest reference.
	retainedRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-launcher", "desktop/darwin/arm64", 11)
	retainedRef.GetManifestRef().BucketId = "missing-retained-bucket"
	const retainedRefKey = "release/manifests/spacewave-launcher/desktop/darwin/arm64/fixable-retained"
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, retainedRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		bus:    tb.Bus,
		conf:   &Config{},
		objKey: objKey,
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-launcher",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected launcher manifest store object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state from readable launcher candidate")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected missing-bucket retained ref not to replace readable launcher candidate")
	}
	status := ctrl.PluginStatus()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected one plugin status, got %d", len(status.Plugins))
	}
	lastError := status.Plugins[0].GetLastErrorMessage()
	if !strings.Contains(lastError, "startup manifest refs: 1 skipped startup manifest ref(s)") {
		t.Fatalf("unexpected retained-ref diagnostic: %q", lastError)
	}
	if !strings.Contains(lastError, "bucket=missing-retained-bucket") {
		t.Fatalf("retained-ref diagnostic %q does not mention missing bucket", lastError)
	}

	// Retain the manifest reference.
	retainedRef.GetManifestRef().BucketId = tb.BucketId
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)

	// Process the manifest World state.
	wait, _, err = pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}

	// The watch loop must wait for changes after the bucket fix.
	if !wait {
		t.Fatal("expected watch loop to wait for changes after bucket fix")
	}

	// Check the execute plugin routine state.
	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected readable launcher candidate to remain selected after bucket fix")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected fixed lower-rev retained ref not to replace readable launcher candidate")
	}
	status = ctrl.PluginStatus()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected existing plugin status to remain, got %d", len(status.Plugins))
	}
	if lastError = status.Plugins[0].GetLastErrorMessage(); lastError != "" {
		t.Fatalf("expected startup manifest skip status to clear after bucket fix, got %q", lastError)
	}
}

func TestWatchWorldManifestLauncherStartsAfterPruningUnavailableRetainedReleaseRef(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "spacewave/launcher"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store the launcher manifest reference.
	launcherRef, launcherRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-launcher", "desktop/darwin/arm64", 12)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, launcherRefKey, "spacewave-launcher")); err != nil {
		t.Fatal(err.Error())
	}

	// Retain the manifest reference.
	retainedRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-launcher", "desktop/darwin/arm64", 13)
	retainedRef.GetManifestRef().BucketId = "spacewave-cdn-release-retained"
	const retainedRefKey = "release/manifests/spacewave-launcher/desktop/darwin/arm64/cdn-retained"
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)
	retainedEdge := bldr_manifest_world.NewManifestQuad(objKey, retainedRefKey, "")
	if err := ws.SetGraphQuad(ctx, retainedEdge); err != nil {
		t.Fatal(err.Error())
	}

	// Collect the manifests for the controller.
	got, errs, err := bldr_manifest_world.CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-launcher",
		[]string{"desktop/darwin/arm64"},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(got) != 1 {
		t.Fatalf("pre-prune manifest count = %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("pre-prune manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), retainedRefKey) {
		t.Fatalf("pre-prune error %q does not mention retained ref key %q", errs[0].Error(), retainedRefKey)
	}
	if !strings.Contains(errs[0].Error(), "spacewave-cdn-release-retained") {
		t.Fatalf("pre-prune error %q does not mention missing retained bucket", errs[0].Error())
	}

	// Assert the expected state.
	if err := ws.DeleteGraphQuad(ctx, retainedEdge); err != nil {
		t.Fatal(err.Error())
	}
	deleted, err := ws.DeleteObject(ctx, retainedRefKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !deleted {
		t.Fatal("expected retained release ref object to be deleted")
	}
	objectState, ok, err := ws.GetObject(ctx, launcherRefKey)
	world.ReleaseObjectState(objectState)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected launcher manifest object to remain after pruning retained ref")
	}

	// Collect the manifests for the controller.
	got, errs, err = bldr_manifest_world.CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-launcher",
		[]string{"desktop/darwin/arm64"},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("post-prune manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("post-prune manifest count = %d", len(got))
	}
	if !got[0].ManifestRef.EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected post-prune startup discovery to keep the readable launcher ref")
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		bus:    tb.Bus,
		conf:   &Config{},
		objKey: objKey,
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-launcher",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected launcher manifest store object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected launcher execute state after pruning unavailable retained ref")
	}
	if execState.pluginHost != host {
		t.Fatal("expected execute state to use matching plugin host")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected pruned copied state to execute the readable launcher candidate")
	}
	status := ctrl.PluginStatus()
	if len(status.Plugins) != 0 {
		t.Fatalf("expected no skipped-ref status after pruning, got %d plugin statuses", len(status.Plugins))
	}
}

func TestWatchWorldManifestRecordsCompactSkippedRefStatusWhenNoCandidate(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest reference.
	badRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/darwin/arm64", 9)
	badRef.GetManifestRef().RootRef.Hash.Hash[0] ^= 0xff
	const badRefKey = "plugin-host/ref/missing"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, badRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		bus:    tb.Bus,
		conf:   &Config{},
		objKey: objKey,
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}
	if pi.executePluginRoutine.GetState() != nil {
		t.Fatal("expected execute state to remain unset")
	}
	if pi.downloadManifestRoutine.GetState() != nil {
		t.Fatal("expected download state to remain unset")
	}

	// Check the plugin status.
	status := ctrl.PluginStatus()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected one plugin status, got %d", len(status.Plugins))
	}
	lastError := status.Plugins[0].GetLastErrorMessage()
	if !strings.Contains(lastError, "1 skipped startup manifest ref(s)") {
		t.Fatalf("unexpected compact skip status: %q", lastError)
	}
	if !strings.Contains(lastError, badRefKey) {
		t.Fatalf("compact skip status %q does not mention bad ref key %q", lastError, badRefKey)
	}
}

func TestWatchWorldManifestSelectsManifestClassPairsByRevision(t *testing.T) {
	const (
		manifestClassLocal   = "local"
		manifestClassNoCopy  = "no-copy"
		manifestClassDynamic = "dynamic-external"
	)
	tests := []struct {
		name              string
		newerClass        string
		olderClass        string
		wantExecuteClass  string
		wantDownloadClass string
	}{
		{
			name:             "newer local over older no-copy",
			newerClass:       manifestClassLocal,
			olderClass:       manifestClassNoCopy,
			wantExecuteClass: manifestClassLocal,
		},
		{
			name:              "newer no-copy over older local",
			newerClass:        manifestClassNoCopy,
			olderClass:        manifestClassLocal,
			wantExecuteClass:  manifestClassNoCopy,
			wantDownloadClass: manifestClassNoCopy,
		},
		{
			name:             "newer local over older dynamic",
			newerClass:       manifestClassLocal,
			olderClass:       manifestClassDynamic,
			wantExecuteClass: manifestClassLocal,
		},
		{
			name:              "newer dynamic over older local",
			newerClass:        manifestClassDynamic,
			olderClass:        manifestClassLocal,
			wantExecuteClass:  manifestClassLocal,
			wantDownloadClass: manifestClassDynamic,
		},
		{
			name:              "newer no-copy over older dynamic",
			newerClass:        manifestClassNoCopy,
			olderClass:        manifestClassDynamic,
			wantExecuteClass:  manifestClassNoCopy,
			wantDownloadClass: manifestClassNoCopy,
		},
		{
			name:              "newer dynamic over older no-copy",
			newerClass:        manifestClassDynamic,
			olderClass:        manifestClassNoCopy,
			wantExecuteClass:  manifestClassNoCopy,
			wantDownloadClass: manifestClassDynamic,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Bound the test with a timeout.
			ctx := context.Background()
			le := logrus.NewEntry(logrus.New())

			// Start the World testbed.
			tb, err := testbed.NewTestbed(ctx, le)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer tb.Release()

			// Build an empty cursor for the World state.
			ocs, err := tb.BuildEmptyCursor(ctx)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer ocs.Release()

			// Build the mock World state.
			ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Name the fixture constants.
			const (
				objKey          = "plugin-host"
				noCopyBucketID  = "dist/project"
				dynamicBucketID = "dynamic-provider"
				manifestID      = "spacewave-core"
				platformID      = "desktop/darwin/arm64"
			)
			if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
				t.Fatal(err.Error())
			}

			// Define the candidate store helper.
			storeCandidate := func(class string, rev uint64) *bldr_manifest.ManifestRef {
				// Declare the fixture state.
				var ref *bldr_manifest.ManifestRef
				var refKey string
				switch class {
				case manifestClassLocal:
					ref, refKey = storeTestWorldManifest(
						t,
						ctx,
						ws,
						manifestID,
						platformID,
						rev,
					)
				case manifestClassNoCopy, manifestClassDynamic:
					bucketID := dynamicBucketID
					if class == manifestClassNoCopy {
						bucketID = noCopyBucketID
					}
					ref = newTestStoredManifestRefInBucket(
						t,
						ctx,
						tb,
						bucketID,
						manifestID,
						platformID,
						rev,
					)
					refKey = "plugin-host/ref/" + class
					storeTestManifestRefObject(t, ctx, ws, refKey, ref)
				default:
					t.Fatalf("unknown manifest class %q", class)
				}
				if err := ws.SetGraphQuad(
					ctx,
					bldr_manifest_world.NewManifestQuad(objKey, refKey, manifestID),
				); err != nil {
					t.Fatal(err.Error())
				}
				return ref
			}

			// Map the fixture manifest refs by id.
			refs := map[string]*bldr_manifest.ManifestRef{
				tt.newerClass: storeCandidate(tt.newerClass, 9),
				tt.olderClass: storeCandidate(tt.olderClass, 7),
			}
			host := &testPluginHost{id: platformID}
			ctrl := &Controller{
				bus: tb.Bus,
				conf: &Config{
					NoCopyBucketIds: []string{noCopyBucketID},
				},
				objKey: objKey,
			}
			pi := &pluginInstance{
				c:                       ctrl,
				le:                      le,
				pluginID:                manifestID,
				manifestCopyStatus:      ccontainer.NewCContainer[*manifestCopyStatus](nil),
				downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
				executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
			}

			// Fetch the object from the World state.
			obj, ok, err := ws.GetObject(ctx, objKey)
			defer world.ReleaseObjectState(obj)
			if err != nil {
				t.Fatal(err.Error())
			}
			if !ok {
				t.Fatal("expected plugin host object")
			}
			wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
				pluginHosts: []bldr_plugin_host.PluginHost{host},
			}, ws, obj)
			if err != nil {
				t.Fatal(err.Error())
			}
			if !wait {
				t.Fatal("expected watch loop to wait for changes")
			}

			// Check the execute plugin routine state.
			execState := pi.executePluginRoutine.GetState()
			if execState == nil || execState.manifestSnapshot == nil {
				t.Fatal("expected executable manifest selection")
			}
			wantExecuteRef := refs[tt.wantExecuteClass].GetManifestRef()
			if !execState.manifestSnapshot.GetManifestRef().EqualVT(wantExecuteRef) {
				t.Fatalf(
					"execute ref = %s, want %s class ref %s",
					execState.manifestSnapshot.GetManifestRef().MarshalString(),
					tt.wantExecuteClass,
					wantExecuteRef.MarshalString(),
				)
			}

			// Check the plugin manifest recovery status.
			recovery := ctrl.pluginStatus.recovery[pluginInstanceKey(manifestID, "")]
			if recovery == nil {
				t.Fatal("expected retained manifest selection status")
			}
			wantDownloadRef := ""
			if tt.wantDownloadClass != "" {
				wantDownloadRef = refs[tt.wantDownloadClass].GetManifestRef().MarshalB58()
			}
			if recovery.DownloadManifestRef != wantDownloadRef {
				t.Fatalf(
					"download candidate = %q, want %s class ref %q",
					recovery.DownloadManifestRef,
					tt.wantDownloadClass,
					wantDownloadRef,
				)
			}

			// Check the download manifest routine state.
			downloadState := pi.downloadManifestRoutine.GetState()
			if tt.wantDownloadClass == manifestClassDynamic {
				wantDynamicRef := refs[manifestClassDynamic].GetManifestRef()
				if downloadState == nil ||
					!downloadState.GetManifestRef().EqualVT(wantDynamicRef) {
					t.Fatalf("download state = %#v, want dynamic manifest", downloadState)
				}
			} else if downloadState != nil {
				t.Fatalf("non-dynamic candidate unexpectedly scheduled a copy: %#v", downloadState)
			}

			// Assert the expected state.
			if tt.wantDownloadClass == manifestClassNoCopy {
				status := pi.manifestCopyStatus.GetValue()
				if status == nil ||
					status.phase != manifestCopyPhaseSuppressed ||
					status.class != manifestCopyClassSuppressed ||
					status.sourceBucketID != noCopyBucketID {
					t.Fatalf("suppressed copy status = %#v", status)
				}
			}
		})
	}
}

func TestWatchWorldManifestFallsBackToBestDownloadWhenNoLocalExecutable(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Use the plugin host object key for the store.
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest reference.
	newerRef := newTestStoredManifestRefInBucket(t, ctx, tb, "remote-bucket", "spacewave-core", "desktop/darwin/arm64", 9)
	const newerRefKey = "plugin-host/ref/remote-newer"
	storeTestManifestRefObject(t, ctx, ws, newerRefKey, newerRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, newerRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest reference.
	olderRef := newTestStoredManifestRefInBucket(t, ctx, tb, "remote-bucket", "spacewave-core", "desktop/darwin/arm64", 7)
	const olderRefKey = "plugin-host/ref/remote-older"
	storeTestManifestRefObject(t, ctx, ws, olderRefKey, olderRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, olderRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	pi := &pluginInstance{
		c: &Controller{
			bus:    tb.Bus,
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Process the manifest World state.
	wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	// Check the download manifest routine state.
	downloadState := pi.downloadManifestRoutine.GetState()
	if downloadState == nil {
		t.Fatal("expected remote manifest to be queued for download")
	}
	if !downloadState.GetManifestRef().EqualVT(newerRef.GetManifestRef()) {
		t.Fatal("expected newest remote manifest to be queued for download")
	}

	// Check the execute plugin routine state.
	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state to fall back to downloadable manifest")
	}
	if execState.pluginHost != host {
		t.Fatal("expected execute state to use matching plugin host")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(newerRef.GetManifestRef()) {
		t.Fatal("expected no-local fallback to select newest downloadable manifest")
	}
}

func TestWatchWorldManifestSkipsUnchangedSelectionInputs(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	baseWS, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, baseWS, objKey); err != nil {
		t.Fatal(err.Error())
	}
	var worldBucketID string
	if err := baseWS.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		worldBucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest reference.
	coreRef := newTestStoredManifestRefWithDistInBucket(
		t,
		ctx,
		tb,
		worldBucketID,
		"spacewave-core",
		"desktop/darwin/arm64",
		1,
	)

	// Store the core manifest in the World.
	coreKey := bldr_manifest.NewManifestArtifactKey(coreRef.GetManifestRef())
	if err := bldr_manifest_world.ExStoreManifestOp(ctx, baseWS, peer.ID("test"), coreKey, []string{objKey}, coreRef); err != nil {
		t.Fatal(err.Error())
	}

	// Wrap the World state with access counting.
	ws := &accessCountingWorldState{WorldState: baseWS}

	// Fetch the plugin host object.
	obj, ok, err := baseWS.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Build the plugin instance fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	hostSet := &pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host}}
	pi := &pluginInstance{
		c: &Controller{
			bus:    tb.Bus,
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// The initial selection must access the World once.
	if _, _, err := pi.processManifestWorldState(ctx, le, hostSet, ws, obj); err != nil {
		t.Fatal(err.Error())
	}
	if got := ws.total.Load(); got != 1 {
		t.Fatalf("initial selection world accesses = %d, want 1", got)
	}

	// Store a test manifest reference.
	webRef := newTestStoredManifestRefWithDistInBucket(
		t,
		ctx,
		tb,
		worldBucketID,
		"spacewave-web",
		"desktop/darwin/arm64",
		1,
	)
	webKey := bldr_manifest.NewManifestArtifactKey(webRef.GetManifestRef())
	if err := bldr_manifest_world.ExStoreManifestOp(ctx, baseWS, peer.ID("test"), webKey, []string{objKey}, webRef); err != nil {
		t.Fatal(err.Error())
	}

	// Fetch the plugin host object again.
	obj, ok, err = baseWS.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object after unrelated manifest store")
	}
	if _, _, err := pi.processManifestWorldState(ctx, le, hostSet, ws, obj); err != nil {
		t.Fatal(err.Error())
	}
	if got := ws.total.Load(); got != 1 {
		t.Fatalf("unchanged selection world accesses = %d, want 1", got)
	}
}

func TestWatchWorldManifestReprocessesReplacementHostWithSamePlatform(t *testing.T) {
	// Bound the test with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}
	manifest, key := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 1)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, key, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Fetch the plugin host object.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Create the plugin host fixture.
	host1 := &testPluginHost{id: "desktop/darwin/arm64"}
	hostSet1 := &pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host1}}
	pi := &pluginInstance{
		c: &Controller{
			bus:    tb.Bus,
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	if _, _, err := pi.processManifestWorldState(ctx, le, hostSet1, ws, obj); err != nil {
		t.Fatal(err.Error())
	}
	initial := pi.executePluginRoutine.GetState()
	if initial == nil || initial.pluginHost != host1 {
		t.Fatal("expected first host to be selected")
	}
	if !initial.manifestSnapshot.GetManifestRef().EqualVT(manifest.GetManifestRef()) {
		t.Fatal("expected first manifest to be selected")
	}

	// Create the plugin host fixture.
	host2 := &testPluginHost{id: "desktop/darwin/arm64"}
	if _, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host2},
	}, ws, obj); err != nil {
		t.Fatal(err.Error())
	}
	replaced := pi.executePluginRoutine.GetState()
	if replaced == nil || replaced.pluginHost != host2 {
		t.Fatal("replacement host with the same platform was not selected")
	}
}

func TestWatchWorldManifestReprocessesNestedSelectionGraphChange(t *testing.T) {
	// Bound the test with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newSchedulerTestWorld(t, ctx, le)
	defer releaseWorldStorage()

	// Build the mock World state.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Name the plugin host and nested object keys.
	const objKey = "plugin-host"
	const nestedKey = "plugin-host/retained"

	// Create the manifest stores and link them.
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, nestedKey); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, nestedKey, "")); err != nil {
		t.Fatal(err.Error())
	}
	first, firstKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 1)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(nestedKey, firstKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Fetch the plugin host object.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	// Create the plugin host fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	hostSet := &pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host}}
	pi := &pluginInstance{
		c: &Controller{
			bus:    tb.Bus,
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	if _, _, err := pi.processManifestWorldState(ctx, le, hostSet, ws, obj); err != nil {
		t.Fatal(err.Error())
	}
	initial := pi.executePluginRoutine.GetState()
	if initial == nil || !initial.manifestSnapshot.GetManifestRef().EqualVT(first.GetManifestRef()) {
		t.Fatal("expected first nested manifest to be selected")
	}

	// Store a manifest in the test World.
	second, secondKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 2)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(nestedKey, secondKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}
	if _, _, err := pi.processManifestWorldState(ctx, le, hostSet, ws, obj); err != nil {
		t.Fatal(err.Error())
	}
	updated := pi.executePluginRoutine.GetState()
	if updated == nil || !updated.manifestSnapshot.GetManifestRef().EqualVT(second.GetManifestRef()) {
		t.Fatal("nested manifest graph change did not update selection")
	}
}
