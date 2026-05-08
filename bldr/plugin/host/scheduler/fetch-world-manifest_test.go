package plugin_host_scheduler

import (
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/routine"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
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
