package plugin_host_scheduler

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	configset "github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/routine"
	"github.com/blang/semver/v4"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	"github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestDirectFetchHandlerPreservesCurrentStateAcrossEmptyGap(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	host1 := &testPluginHost{id: "desktop/linux/amd64"}
	host2 := &testPluginHost{id: "desktop/linux/amd64"}
	pi := &pluginInstance{
		c: &Controller{
			conf: &Config{},
		},
		le:                      le,
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(&pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host1}})

	val1 := bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 1, "bucket-1"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host1 || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state to be set from first fetched manifest")
	}
	if execState.manifestSnapshot.GetManifestRef() == nil {
		t.Fatal("expected manifest snapshot ref to be set")
	}

	handler.HandleValueRemoved(nil, directive.NewAttachedValue(1, val1))

	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host1 {
		t.Fatal("expected execute state to remain during empty fetch-manifest gap")
	}
	if pi.downloadManifestRoutine.GetState() == nil {
		t.Fatal("expected download manifest state to remain during empty fetch-manifest gap")
	}
	originalExecState := execState

	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	execState = pi.executePluginRoutine.GetState()
	if execState != originalExecState {
		t.Fatal("expected re-adding the same manifest target to avoid resetting execute state")
	}

	val2 := bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 2, "bucket-2"),
	})
	handler = pi.newDirectFetchHandler(&pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host2}})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(2, val2))

	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != host2 {
		t.Fatal("expected execute state to update to replacement plugin host")
	}
	if execState.manifestSnapshot.GetManifestRef() == nil {
		t.Fatal("expected replacement manifest snapshot ref to be set")
	}
}

func TestDirectFetchHandlerPrefersCurrentStateAcrossEqualRevOverlap(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	host := &testPluginHost{id: "desktop/linux/amd64"}
	pi := &pluginInstance{
		c: &Controller{
			conf: &Config{},
		},
		le:                      le,
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(&pluginHostSet{pluginHosts: []bldr_plugin_host.PluginHost{host}})

	val1 := bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 7, "bucket-a"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, val1))

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state after first manifest")
	}
	firstRef := execState.manifestSnapshot.GetManifestRef()
	if firstRef == nil {
		t.Fatal("expected first manifest ref")
	}

	val2 := bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-app", "desktop/linux/amd64", 7, "bucket-b"),
	})
	handler.HandleValueAdded(nil, directive.NewAttachedValue(2, val2))

	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state during equal-rev overlap")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(firstRef) {
		t.Fatal("expected equal-rev overlap to preserve the current execute target")
	}

	handler.HandleValueRemoved(nil, directive.NewAttachedValue(1, val1))

	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state after removing original candidate")
	}
	if execState.manifestSnapshot.GetManifestRef().EqualVT(firstRef) {
		t.Fatal("expected execute target to switch once the original candidate is removed")
	}
}

func TestExecutePluginArgsEqualHandlesNilManifestRefs(t *testing.T) {
	if !executePluginArgsEqual(
		&executePluginArgs{manifestSnapshot: &bldr_manifest.ManifestSnapshot{}},
		&executePluginArgs{manifestSnapshot: &bldr_manifest.ManifestSnapshot{}},
	) {
		t.Fatal("expected args with nil manifest refs to compare equal")
	}

	withRef := &executePluginArgs{
		manifestSnapshot: &bldr_manifest.ManifestSnapshot{
			ManifestRef: &bucket.ObjectRef{BucketId: "bucket"},
		},
	}
	withoutRef := &executePluginArgs{manifestSnapshot: &bldr_manifest.ManifestSnapshot{}}
	if executePluginArgsEqual(withRef, withoutRef) {
		t.Fatal("expected args with one nil manifest ref to differ")
	}
}

func TestDirectFetchCandidateBetterPrefersNativePlatform(t *testing.T) {
	js := &directFetchCandidate{
		ref:  newTestManifestRef("spacewave-v86", "js", 68, "bucket"),
		host: &testPluginHost{id: "js"},
	}
	web := &directFetchCandidate{
		ref:  newTestManifestRef("spacewave-v86", "web/js/wasm", 0, "bucket"),
		host: &testPluginHost{id: "web/js/wasm"},
	}

	if !directFetchCandidateBetter(web, js) {
		t.Fatal("native browser platform should outrank js fallback even when fallback rev is higher")
	}
	if directFetchCandidateBetter(js, web) {
		t.Fatal("js fallback should not outrank native browser platform")
	}
}

func TestFilterPluginPlatformIDsHonorsPlatformPolicy(t *testing.T) {
	conf := webPlatformAllowlistConfig("spacewave-v86")
	got := conf.FilterPluginPlatformIDs("spacewave-core", []string{
		"js",
		"web/js/wasm",
		"desktop/darwin/arm64",
	})
	want := []string{"js", "desktop/darwin/arm64"}
	if len(got) != len(want) {
		t.Fatalf("platform ids: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("platform ids: got %v, want %v", got, want)
		}
	}

	got = conf.FilterPluginPlatformIDs("spacewave-v86", []string{"js", "web/js/wasm"})
	want = []string{"js", "web/js/wasm"}
	if len(got) != len(want) {
		t.Fatalf("allowed platform ids: got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("allowed platform ids: got %v, want %v", got, want)
		}
	}
}

func TestDirectFetchHandlerFiltersWebPlatformForUnlistedPlugin(t *testing.T) {
	le := logrus.NewEntry(logrus.New())
	webHost := &testPluginHost{id: "web/js/wasm"}
	jsHost := &testPluginHost{id: "js"}
	pi := &pluginInstance{
		c: &Controller{
			conf: webPlatformAllowlistConfig("spacewave-v86"),
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	handler := pi.newDirectFetchHandler(&pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{jsHost, webHost},
	})

	handler.HandleValueAdded(nil, directive.NewAttachedValue(1, bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{
		newTestManifestRef("spacewave-core", "web/js/wasm", 99, "bucket-web"),
		newTestManifestRef("spacewave-core", "js", 1, "bucket-js"),
	})))

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != jsHost {
		t.Fatal("expected unlisted plugin to use js fallback instead of web/js/wasm")
	}
}

func TestFetchManifestValueStorerRepairsMissingManifestLink(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	ref := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/darwin/arm64", 1)
	manifestKey := bldr_manifest.NewManifestKey(objKey, ref.GetMeta())
	if _, _, err := bldr_manifest_world.SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-core",
		[]string{"desktop/darwin/arm64"},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 0 {
		t.Fatalf("expected orphaned manifest to be unreachable, got %d", len(got))
	}

	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			objKey:        objKey,
			peerID:        peer.ID("test"),
			worldStateCtr: ccontainer.NewCContainer(wsv),
		},
		le: le,
	}
	storer := &fetchManifestValueStorer{
		pi:     pi,
		value:  promise.NewPromiseWithResult(bldr_manifest.NewFetchManifestValue([]*bldr_manifest.ManifestRef{ref}), nil),
		refIdx: 0,
	}
	if err := storer.execFetchManifestValueStorer(ctx); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err = bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-core",
		[]string{"desktop/darwin/arm64"},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("expected repaired manifest link, got %d", len(got))
	}
	if !got[0].ManifestRef.EqualVT(ref.GetManifestRef()) {
		t.Fatal("manifest ref changed during repair")
	}
}

func TestWatchWorldManifestUsesStartupManifestRefsAndSkipsBadCandidate(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	goodRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/darwin/arm64", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, goodRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	badRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/darwin/arm64", 9)
	badRef.GetManifestRef().RootRef.Hash.Hash[0] ^= 0xff
	const badRefKey = "plugin-host/ref/missing"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, badRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	pi := &pluginInstance{
		c: &Controller{
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

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
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	webRef, webRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "web/js/wasm", 99)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, webRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}
	jsRef, jsRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "js", 1)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, jsRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}
	_ = webRef

	webHost := &testPluginHost{id: "web/js/wasm"}
	jsHost := &testPluginHost{id: "js"}
	pi := &pluginInstance{
		c: &Controller{
			conf:   webPlatformAllowlistConfig("spacewave-v86"),
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{jsHost, webHost},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.pluginHost != jsHost {
		t.Fatal("expected unlisted startup plugin to use js fallback")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(jsRef.GetManifestRef()) {
		t.Fatal("expected unlisted startup plugin not to select web/js/wasm manifest")
	}
}

func TestWatchWorldManifestExecutesBootstrapManifestAndRecordsUnreadableRetainedRef(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	bootstrapRef, bootstrapRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 7)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, bootstrapRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	retainedRef := newTestStoredManifestRef(t, ctx, tb, "other-plugin", "desktop/darwin/arm64", 9)
	const retainedRefKey = "plugin-host/ref/unreadable-retained"
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)
	corruptTestWorldObjectRoot(t, ctx, ws, retainedRefKey)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, retainedRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		conf:   &Config{},
		objKey: objKey,
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

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

	status := ctrl.GetPluginStatusCtr().GetValue()
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
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "spacewave/launcher"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	launcherRef, launcherRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-launcher", "desktop/darwin/arm64", 12)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, launcherRefKey, "spacewave-launcher")); err != nil {
		t.Fatal(err.Error())
	}

	retainedRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-launcher", "desktop/darwin/arm64", 13)
	retainedRef.GetManifestRef().BucketId = "spacewave-cdn-release-retained"
	const retainedRefKey = "release/manifests/spacewave-launcher/desktop/darwin/arm64/cdn-retained"
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)
	retainedEdge := bldr_manifest_world.NewManifestQuad(objKey, retainedRefKey, "")
	if err := ws.SetGraphQuad(ctx, retainedEdge); err != nil {
		t.Fatal(err.Error())
	}

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		conf:   &Config{},
		objKey: objKey,
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-launcher",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected launcher manifest store object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

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

	status := ctrl.GetPluginStatusCtr().GetValue()
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

	wait, err = pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes after pruning")
	}
	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected readable launcher candidate to remain selected after pruning")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected pruned scan to keep the readable launcher candidate")
	}
	status = ctrl.GetPluginStatusCtr().GetValue()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected existing plugin status to remain, got %d", len(status.Plugins))
	}
	if lastError = status.Plugins[0].GetLastErrorMessage(); lastError != "" {
		t.Fatalf("expected startup manifest skip status to clear after pruning, got %q", lastError)
	}
}

func TestWatchWorldManifestClearsSkippedRefStatusAfterBucketFix(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "spacewave/launcher"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	launcherRef, launcherRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-launcher", "desktop/darwin/arm64", 12)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, launcherRefKey, "spacewave-launcher")); err != nil {
		t.Fatal(err.Error())
	}

	retainedRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-launcher", "desktop/darwin/arm64", 11)
	retainedRef.GetManifestRef().BucketId = "missing-retained-bucket"
	const retainedRefKey = "release/manifests/spacewave-launcher/desktop/darwin/arm64/fixable-retained"
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, retainedRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		conf:   &Config{},
		objKey: objKey,
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-launcher",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected launcher manifest store object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state from readable launcher candidate")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected missing-bucket retained ref not to replace readable launcher candidate")
	}
	status := ctrl.GetPluginStatusCtr().GetValue()
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

	retainedRef.GetManifestRef().BucketId = tb.BucketId
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)

	wait, err = pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes after bucket fix")
	}
	execState = pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected readable launcher candidate to remain selected after bucket fix")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(launcherRef.GetManifestRef()) {
		t.Fatal("expected fixed lower-rev retained ref not to replace readable launcher candidate")
	}
	status = ctrl.GetPluginStatusCtr().GetValue()
	if len(status.Plugins) != 1 {
		t.Fatalf("expected existing plugin status to remain, got %d", len(status.Plugins))
	}
	if lastError = status.Plugins[0].GetLastErrorMessage(); lastError != "" {
		t.Fatalf("expected startup manifest skip status to clear after bucket fix, got %q", lastError)
	}
}

func TestWatchWorldManifestLauncherStartsAfterPruningUnavailableRetainedReleaseRef(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "spacewave/launcher"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	launcherRef, launcherRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-launcher", "desktop/darwin/arm64", 12)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, launcherRefKey, "spacewave-launcher")); err != nil {
		t.Fatal(err.Error())
	}

	retainedRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-launcher", "desktop/darwin/arm64", 13)
	retainedRef.GetManifestRef().BucketId = "spacewave-cdn-release-retained"
	const retainedRefKey = "release/manifests/spacewave-launcher/desktop/darwin/arm64/cdn-retained"
	storeTestManifestRefObject(t, ctx, ws, retainedRefKey, retainedRef)
	retainedEdge := bldr_manifest_world.NewManifestQuad(objKey, retainedRefKey, "")
	if err := ws.SetGraphQuad(ctx, retainedEdge); err != nil {
		t.Fatal(err.Error())
	}

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
	_, ok, err := ws.GetObject(ctx, launcherRefKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected launcher manifest object to remain after pruning retained ref")
	}

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

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		conf:   &Config{},
		objKey: objKey,
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-launcher",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected launcher manifest store object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

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
	status := ctrl.GetPluginStatusCtr().GetValue()
	if len(status.Plugins) != 0 {
		t.Fatalf("expected no skipped-ref status after pruning, got %d plugin statuses", len(status.Plugins))
	}
}

func TestWatchWorldManifestRecordsCompactSkippedRefStatusWhenNoCandidate(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	badRef := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/darwin/arm64", 9)
	badRef.GetManifestRef().RootRef.Hash.Hash[0] ^= 0xff
	const badRefKey = "plugin-host/ref/missing"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, badRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	ctrl := &Controller{
		conf:   &Config{},
		objKey: objKey,
		pluginStatusCtr: ccontainer.NewCContainerWithEqual(
			&PluginStatusSnapshot{},
			pluginStatusSnapshotEqual,
		),
		pluginStatus: make(map[string]*bldr_plugin.PluginStatus),
	}
	pi := &pluginInstance{
		c:                       ctrl,
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
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

	status := ctrl.GetPluginStatusCtr().GetValue()
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

func TestWatchWorldManifestPrefersLocalExecutableOverNewerDownload(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	remoteRef := newTestStoredManifestRefInBucket(t, ctx, tb, "remote-bucket", "spacewave-core", "desktop/darwin/arm64", 9)
	const remoteRefKey = "plugin-host/ref/remote-newer"
	storeTestManifestRefObject(t, ctx, ws, remoteRefKey, remoteRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, remoteRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	localRef, localRefKey := storeTestWorldManifest(t, ctx, ws, "spacewave-core", "desktop/darwin/arm64", 7)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, localRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	pi := &pluginInstance{
		c: &Controller{
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	downloadState := pi.downloadManifestRoutine.GetState()
	if downloadState == nil {
		t.Fatal("expected newer remote manifest to be queued for download")
	}
	if !downloadState.GetManifestRef().EqualVT(remoteRef.GetManifestRef()) {
		t.Fatal("expected download state to use newest remote manifest")
	}

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected execute state from local manifest")
	}
	if execState.pluginHost != host {
		t.Fatal("expected execute state to use matching plugin host")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(localRef.GetManifestRef()) {
		t.Fatal("expected local executable manifest to remain selected while newer remote downloads")
	}
}

func TestWatchWorldManifestFallsBackToBestDownloadWhenNoLocalExecutable(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	newerRef := newTestStoredManifestRefInBucket(t, ctx, tb, "remote-bucket", "spacewave-core", "desktop/darwin/arm64", 9)
	const newerRefKey = "plugin-host/ref/remote-newer"
	storeTestManifestRefObject(t, ctx, ws, newerRefKey, newerRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, newerRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	olderRef := newTestStoredManifestRefInBucket(t, ctx, tb, "remote-bucket", "spacewave-core", "desktop/darwin/arm64", 7)
	const olderRefKey = "plugin-host/ref/remote-older"
	storeTestManifestRefObject(t, ctx, ws, olderRefKey, olderRef)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, olderRefKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	pi := &pluginInstance{
		c: &Controller{
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host object")
	}

	wait, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !wait {
		t.Fatal("expected watch loop to wait for changes")
	}

	downloadState := pi.downloadManifestRoutine.GetState()
	if downloadState == nil {
		t.Fatal("expected remote manifest to be queued for download")
	}
	if !downloadState.GetManifestRef().EqualVT(newerRef.GetManifestRef()) {
		t.Fatal("expected newest remote manifest to be queued for download")
	}

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

func TestProcessManifestWorldStateRunsDownloadAndExecuteForRemoteManifest(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	const remoteBucketID = "remote-manifest-bucket"
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  remoteBucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}
	ref := newTestStoredManifestRefInBucket(t, ctx, tb, remoteBucketID, "spacewave-core", "desktop/darwin/arm64", 2)
	var worldBucketID string
	if err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		worldBucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
	if ref.GetManifestRef().GetBucketId() == worldBucketID {
		t.Fatal("test manifest must start in a non-local bucket")
	}

	manifestKey := bldr_manifest.NewManifestKey(objKey, ref.GetMeta())
	if err := bldr_manifest_world.ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{objKey}, ref); err != nil {
		t.Fatal(err.Error())
	}

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host manifest store object")
	}

	host := &testPluginHost{id: "desktop/darwin/arm64"}
	pi := &pluginInstance{
		c: &Controller{
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	waitForChanges, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !waitForChanges {
		t.Fatal("expected world manifest watch to continue")
	}

	downloadState := pi.downloadManifestRoutine.GetState()
	if downloadState == nil {
		t.Fatal("expected remote manifest to schedule background DAG copy")
	}
	if !downloadState.GetManifestRef().EqualVT(ref.GetManifestRef()) {
		t.Fatal("download manifest ref changed")
	}

	execState := pi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected remote manifest to be executable while copy runs")
	}
	if execState.pluginHost != host {
		t.Fatal("expected execute state to use matching plugin host")
	}
	if !execState.manifestSnapshot.GetManifestRef().EqualVT(ref.GetManifestRef()) {
		t.Fatal("execute manifest ref changed")
	}
}

func TestExecPluginReadsExternalManifestViaLookupBlockFromNetwork(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	const lookupBucketID = "release-world-cdn-bucket"
	bucketLkConfig, err := bucket.NewLookupConfig(configset.NewControllerConfig(1, &lookup_concurrent.Config{
		NotFoundBehavior: lookup_concurrent.NotFoundBehavior_NotFoundBehavior_LOOKUP_DIRECTIVE,
	}))
	if err != nil {
		t.Fatal(err.Error())
	}
	bucketConf, err := bucket.NewConfig(lookupBucketID, 1, bucketLkConfig)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, bucketConf); err != nil {
		t.Fatal(err.Error())
	}

	remote := newTestExternalManifestRefWithDistAssets(t, ctx, lookupBucketID, "spacewave-core", "desktop/darwin/arm64", 3)
	remoteStore := block_store.NewStore("test/release-world-cdn", remote.store)
	storeCtrl := block_store_controller.NewController(
		le,
		controller.NewInfo("test/release-world-cdn-store", semver.MustParse("0.0.1"), ""),
		block_store_controller.NewBlockStoreBuilder(remoteStore),
		nil,
		true,
		[]string{lookupBucketID},
		true,
		false,
	)
	storeRel, err := tb.Bus.AddController(ctx, storeCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer storeRel()

	const refKey = "plugin-host/ref/release-world-cdn"
	storeTestManifestRefObject(t, ctx, ws, refKey, remote.ref)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, refKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := bldr_manifest_world.CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-core",
		[]string{"desktop/darwin/arm64"},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(got) != 0 {
		t.Fatalf("startup discovery selected external CDN ref, got %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("startup discovery errors = %v", errs)
	}
	if remote.store.gets.Load() != 0 {
		t.Fatal("startup local-only discovery should not invoke LookupBlockFromNetwork")
	}

	host := &releaseCDNRuntimePluginHost{
		testPluginHost: testPluginHost{id: "desktop/darwin/arm64"},
	}
	hostCtrl := plugin_host_controller.NewController(
		le,
		tb.Bus,
		controller.NewInfo("test/plugin-host", semver.MustParse("0.0.1"), ""),
		host,
	)
	hostRel, err := tb.Bus.AddController(ctx, hostCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer hostRel()

	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus:           tb.Bus,
			conf:          &Config{},
			objKey:        objKey,
			worldStateCtr: ccontainer.NewCContainer(wsv),
			hostVolumeCtr: ccontainer.NewCContainer(&hostVol{
				vol: tb.Volume,
				info: &volume.VolumeInfo{
					VolumeId: tb.Volume.GetID(),
				},
			}),
			pluginStatusCtr: ccontainer.NewCContainer(&PluginStatusSnapshot{}),
			pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
		},
		le:               le,
		pluginID:         "spacewave-core",
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
	}
	if err := pi.execPlugin(ctx, &executePluginArgs{
		manifestSnapshot: &bldr_manifest.ManifestSnapshot{
			ManifestRef: remote.ref.GetManifestRef(),
			Manifest:    remote.manifest,
		},
		pluginHost: host,
	}); err != nil {
		t.Fatal(err.Error())
	}
	if remote.store.gets.Load() == 0 {
		t.Fatal("expected demand execution to invoke LookupBlockFromNetwork")
	}
	if string(host.distData) != "console.log('release cdn')\n" {
		t.Fatalf("dist entrypoint bytes = %q", host.distData)
	}
	if string(host.assetsData) != "release asset\n" {
		t.Fatalf("asset bytes = %q", host.assetsData)
	}
}

func TestDownloadManifestCopiesRemoteDAGAndStoresLocalWorldRef(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	const remoteBucketID = "remote-manifest-bucket"
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  remoteBucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}
	ref := newTestStoredManifestRefWithDistInBucket(t, ctx, tb, remoteBucketID, "spacewave-core", "desktop/darwin/arm64", 2)
	var worldBucketID string
	if err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		worldBucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
	if ref.GetManifestRef().GetBucketId() == worldBucketID {
		t.Fatal("test manifest must start in a non-local bucket")
	}

	manifestKey := bldr_manifest.NewManifestKey(objKey, ref.GetMeta())
	if err := bldr_manifest_world.ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{objKey}, ref); err != nil {
		t.Fatal(err.Error())
	}

	// Simulate the startup execute path reading the remote manifest before the
	// background copy gets worker time.
	var remoteManifest *bldr_manifest.Manifest
	if err := bldr_manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, ref.GetManifestRef(), func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
		remoteManifest = manifest.CloneVT()
		_, _, err := distFS.LookupPath(ctx, manifest.GetEntrypoint())
		return err
	}); err != nil {
		t.Fatal(err.Error())
	}
	if remoteManifest == nil {
		t.Fatal("expected remote manifest to be decoded")
	}

	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			conf:            &Config{},
			objKey:          objKey,
			peerID:          peer.ID("test"),
			worldStateCtr:   ccontainer.NewCContainer(wsv),
			pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
			pluginStatusCtr: ccontainer.NewCContainer(&PluginStatusSnapshot{}),
		},
		le:       le,
		pluginID: "spacewave-core",
	}
	if err := pi.execDownloadManifest(ctx, &bldr_manifest.ManifestSnapshot{
		ManifestRef: ref.GetManifestRef(),
		Manifest:    remoteManifest,
	}); err != nil {
		t.Fatal(err.Error())
	}

	got, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-core",
		[]string{"desktop/darwin/arm64"},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d, want 1", len(got))
	}
	localRef := got[0].ManifestRef
	if localRef.GetBucketId() != worldBucketID {
		t.Fatalf("manifest bucket = %q, want local world bucket %q", localRef.GetBucketId(), worldBucketID)
	}
	if !localRef.GetRootRef().EqualVT(ref.GetManifestRef().GetRootRef()) {
		t.Fatal("local manifest root ref changed")
	}
	if !got[0].Manifest.GetMeta().EqualVT(remoteManifest.GetMeta()) {
		t.Fatal("stored local manifest metadata changed")
	}
	if got[0].Manifest.GetEntrypoint() != remoteManifest.GetEntrypoint() {
		t.Fatal("stored local manifest entrypoint changed")
	}
	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host manifest store object")
	}
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	watchPi := &pluginInstance{
		c: &Controller{
			conf:   &Config{},
			objKey: objKey,
		},
		le:                      le,
		pluginID:                "spacewave-core",
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	if _, err := watchPi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj); err != nil {
		t.Fatal(err.Error())
	}
	if downloadState := watchPi.downloadManifestRoutine.GetState(); downloadState != nil {
		t.Fatalf("expected local manifest to stop background copy scheduling, got %s", downloadState.GetManifestRef().MarshalString())
	}
	execState := watchPi.executePluginRoutine.GetState()
	if execState == nil || execState.manifestSnapshot == nil {
		t.Fatal("expected local manifest to remain executable")
	}
	if execState.manifestSnapshot.GetManifestRef().GetBucketId() != worldBucketID {
		t.Fatal("expected executable manifest to use local world bucket")
	}
	if err := bldr_manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, localRef, func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
		_, _, err := distFS.LookupPath(ctx, manifest.GetEntrypoint())
		return err
	}); err != nil {
		t.Fatal(err.Error())
	}
}

func TestDownloadManifestRejectsMissingSnapshotMetadataBeforeStore(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	const remoteBucketID = "remote-manifest-bucket"
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  remoteBucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}
	ref := newTestStoredManifestRefWithDistInBucket(t, ctx, tb, remoteBucketID, "spacewave-core", "desktop/darwin/arm64", 2)

	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			conf:            &Config{},
			objKey:          objKey,
			peerID:          peer.ID("test"),
			worldStateCtr:   ccontainer.NewCContainer(wsv),
			pluginStatus:    make(map[string]*bldr_plugin.PluginStatus),
			pluginStatusCtr: ccontainer.NewCContainer(&PluginStatusSnapshot{}),
		},
		le:       le,
		pluginID: "spacewave-core",
	}
	err = pi.execDownloadManifest(ctx, &bldr_manifest.ManifestSnapshot{
		ManifestRef: ref.GetManifestRef(),
	})
	if err == nil {
		t.Fatal("expected missing manifest metadata to fail")
	}
	if !strings.Contains(err.Error(), "manifest snapshot metadata") {
		t.Fatalf("error = %q, want manifest snapshot metadata", err.Error())
	}

	got, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-core",
		[]string{"desktop/darwin/arm64"},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 0 {
		t.Fatalf("manifest count = %d, want 0", len(got))
	}
}

func newTestManifestRef(manifestID, platformID string, rev uint64, bucketID string) *bldr_manifest.ManifestRef {
	return bldr_manifest.NewManifestRef(
		bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_DEV, platformID, rev),
		&bucket.ObjectRef{BucketId: bucketID},
	)
}

func newTestStoredManifestRef(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	manifestID,
	platformID string,
	rev uint64,
) *bldr_manifest.ManifestRef {
	t.Helper()
	return newTestStoredManifestRefInBucket(t, ctx, tb, tb.BucketId, manifestID, platformID, rev)
}

func newTestStoredManifestRefInBucket(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	bucketID,
	manifestID,
	platformID string,
	rev uint64,
) *bldr_manifest.ManifestRef {
	t.Helper()

	meta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_RELEASE, platformID, rev)
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  bucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		bucketID,
		tb.Volume.GetID(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ref := oc.GetRef()
	ref.RootRef = rootRef
	return bldr_manifest.NewManifestRef(meta, ref)
}

func storeTestWorldManifest(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	manifestID,
	platformID string,
	rev uint64,
) (*bldr_manifest.ManifestRef, string) {
	t.Helper()

	meta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_RELEASE, platformID, rev)
	var ref *bucket.ObjectRef
	err := ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		btx, bcs := bls.BuildTransaction(nil)
		bcs.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
		rootRef, _, err := btx.Write(ctx, true)
		if err != nil {
			return err
		}
		ref = &bucket.ObjectRef{
			BucketId: bls.GetOpArgs().GetBucketId(),
			RootRef:  rootRef,
		}
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	objKey := "plugin-host/manifest/" + manifestID + "/" + platformID
	if _, _, err := bldr_manifest_world.SetManifest(ctx, ws, peer.ID("test"), objKey, ref); err != nil {
		t.Fatal(err.Error())
	}
	return bldr_manifest.NewManifestRef(meta, ref), objKey
}

func newTestStoredManifestRefWithDistInBucket(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	bucketID,
	manifestID,
	platformID string,
	rev uint64,
) *bldr_manifest.ManifestRef {
	t.Helper()

	entrypoint := "plugin.js"
	distFS := memfs.New()
	f, err := distFS.Create(entrypoint)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := f.Write([]byte("console.log('startup')\n")); err != nil {
		t.Fatal(err.Error())
	}
	if err := f.Close(); err != nil {
		t.Fatal(err.Error())
	}

	meta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_RELEASE, platformID, rev)
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		bucketID,
		tb.Volume.GetID(),
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	btx, bcs := oc.BuildTransaction(nil)
	if _, err := bldr_manifest.CreateManifestWithBilly(ctx, bcs, meta, entrypoint, distFS, nil, timestamppb.Now()); err != nil {
		t.Fatal(err.Error())
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ref := oc.GetRef()
	ref.RootRef = rootRef
	return bldr_manifest.NewManifestRef(meta, ref)
}

type testExternalManifestRef struct {
	ref      *bldr_manifest.ManifestRef
	manifest *bldr_manifest.Manifest
	store    *countingBlockStore
}

func newTestExternalManifestRefWithDistAssets(
	t *testing.T,
	ctx context.Context,
	bucketID,
	manifestID,
	platformID string,
	rev uint64,
) *testExternalManifestRef {
	t.Helper()

	const entrypoint = "plugin.js"
	distFS := memfs.New()
	f, err := distFS.Create(entrypoint)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := f.Write([]byte("console.log('release cdn')\n")); err != nil {
		t.Fatal(err.Error())
	}
	if err := f.Close(); err != nil {
		t.Fatal(err.Error())
	}

	assetsFS := memfs.New()
	f, err = assetsFS.Create("asset.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := f.Write([]byte("release asset\n")); err != nil {
		t.Fatal(err.Error())
	}
	if err := f.Close(); err != nil {
		t.Fatal(err.Error())
	}

	kvk, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	ops := block_store_kvtx.NewKVTxBlock(kvk, store_kvtx_inmem.NewStore(), 0, true)
	store := &countingBlockStore{store: ops}
	meta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_RELEASE, platformID, rev)
	btx, bcs := block.NewTransaction(store, nil, nil, nil)
	manifest, err := bldr_manifest.CreateManifestWithBilly(ctx, bcs, meta, entrypoint, distFS, assetsFS, timestamppb.Now())
	if err != nil {
		t.Fatal(err.Error())
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	ref := &bucket.ObjectRef{
		BucketId: bucketID,
		RootRef:  rootRef,
	}
	return &testExternalManifestRef{
		ref:      bldr_manifest.NewManifestRef(meta, ref),
		manifest: manifest.CloneVT(),
		store:    store,
	}
}

type countingBlockStore struct {
	store block.StoreOps
	gets  atomic.Uint32
}

func (s *countingBlockStore) GetHashType() hash.HashType {
	return s.store.GetHashType()
}

func (s *countingBlockStore) GetSupportedFeatures() block.StoreFeature {
	return s.store.GetSupportedFeatures()
}

func (s *countingBlockStore) PutBlock(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	return s.store.PutBlock(ctx, data, opts)
}

func (s *countingBlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return s.store.PutBlockBatch(ctx, entries)
}

func (s *countingBlockStore) PutBlockBackground(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	return s.store.PutBlockBackground(ctx, data, opts)
}

func (s *countingBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	s.gets.Add(1)
	return s.store.GetBlock(ctx, ref)
}

func (s *countingBlockStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return s.store.GetBlockExists(ctx, ref)
}

func (s *countingBlockStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return s.store.GetBlockExistsBatch(ctx, refs)
}

func (s *countingBlockStore) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return s.store.RmBlock(ctx, ref)
}

func (s *countingBlockStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return s.store.StatBlock(ctx, ref)
}

func (s *countingBlockStore) Flush(ctx context.Context) error {
	return s.store.Flush(ctx)
}

func (s *countingBlockStore) BeginDeferFlush() {
	s.store.BeginDeferFlush()
}

func (s *countingBlockStore) EndDeferFlush(ctx context.Context) error {
	return s.store.EndDeferFlush(ctx)
}

var _ block.StoreOps = ((*countingBlockStore)(nil))

func storeTestManifestRefObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	ref *bldr_manifest.ManifestRef,
) {
	t.Helper()

	if _, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(ref.CloneVT(), true)
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
}

func corruptTestWorldObjectRoot(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) {
	t.Helper()

	obj, ok, err := ws.GetObject(ctx, objKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatalf("expected object %q", objKey)
	}
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ref == nil || ref.GetRootRef().GetEmpty() {
		t.Fatalf("expected object %q root ref", objKey)
	}
	corruptRef := ref.CloneVT()
	corruptRef.RootRef.Hash.Hash[0] ^= 0xff
	if _, err := obj.SetRootRef(ctx, corruptRef); err != nil {
		t.Fatal(err.Error())
	}
}

func webPlatformAllowlistConfig(pluginIDs ...string) *Config {
	return &Config{
		PlatformSelectionPolicies: []*PlatformSelectionPolicy{
			{
				PlatformId:       "web/js/wasm",
				AllowedPluginIds: pluginIDs,
			},
		},
	}
}

type testPluginHost struct {
	id string
}

func (h *testPluginHost) GetPlatformId() string {
	return h.id
}

func (h *testPluginHost) Execute(ctx context.Context) error {
	return nil
}

func (h *testPluginHost) ListPlugins(ctx context.Context) ([]string, error) {
	return nil, nil
}

func (h *testPluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID,
	instanceKey,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
	return nil
}

func (h *testPluginHost) DeletePlugin(ctx context.Context, pluginID string) error {
	return nil
}

type releaseCDNRuntimePluginHost struct {
	testPluginHost
	distData   []byte
	assetsData []byte
}

func (h *releaseCDNRuntimePluginHost) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}

func (h *releaseCDNRuntimePluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID,
	instanceKey,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
	distFile, _, err := pluginDist.LookupPath(ctx, entrypoint)
	if err != nil {
		if distFile != nil {
			distFile.Release()
		}
		return err
	}
	defer distFile.Release()
	h.distData, err = unixfs.ReadFile(ctx, distFile)
	if err != nil {
		return err
	}

	assetFile, _, err := pluginAssets.LookupPath(ctx, "asset.txt")
	if err != nil {
		if assetFile != nil {
			assetFile.Release()
		}
		return err
	}
	defer assetFile.Release()
	h.assetsData, err = unixfs.ReadFile(ctx, assetFile)
	return err
}
