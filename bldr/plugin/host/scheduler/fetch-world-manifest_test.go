package plugin_host_scheduler

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
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
