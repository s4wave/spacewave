package plugin_host_scheduler

import (
	"bytes"
	"context"
	"errors"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/routine"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_materializer "github.com/s4wave/spacewave/bldr/manifest/materializer"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	bldr_plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	block "github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	bucket "github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	"github.com/sirupsen/logrus"
)

// TestMaterializeManifestThroughPluginHost exercises browser worker demand,
// copy completion, and release with and without an instance key, and that
// every block the plugin copies is held by the host's stage.
func TestMaterializeManifestThroughPluginHost(t *testing.T) {
	t.Run("empty instance key", func(t *testing.T) {
		runBrowserCopyFixture(t, "")
	})
	t.Run("copy-instance", func(t *testing.T) {
		runBrowserCopyFixture(t, "copy-instance")
	})
}

// runBrowserCopyFixture copies a manifest from a source testbed to a host
// testbed through the real Materializer service running on a third plugin
// bus, exercising the production wiring end to end: request-scoped source and
// destination BlockStore RPC (plugin-scoped server filter), plugin-host/
// prefix routing, and the destination ProxyVolume mapping the plugin mounts.
// The host Volume frees, so it refuses a put nothing holds: the copy succeeds
// only if every block the plugin writes lands under the host's stage.
// instanceKey selects the browser worker identity the store filter must
// accept.
func runBrowserCopyFixture(t *testing.T, instanceKey string) {
	// Bind the test context and logger.
	t.Helper()
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())

	// Three independent buses: source store, plugin host, plugin worker.
	srcTB, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(srcTB.Release)

	// Build the host and plugin worker testbeds. The host Volume frees, so
	// it refuses an unheld put.
	hostTB, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_kvtxinmem.Config{
		VolumeConfig: &volume_controller.Config{},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"},
	}))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(hostTB.Release)
	pluginTB, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(pluginTB.Release)

	// Buckets: source on the source bus, destination on the host bus.
	for _, cfg := range []struct {
		tb       *testbed.Testbed
		bucketID string
	}{{srcTB, "mm-src"}, {hostTB, "mm-dest"}} {
		if _, _, _, err := cfg.tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
			Id:  cfg.bucketID,
			Rev: 1,
		}); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Source cursor with gzip transform; destination cursor with s2.
	srcTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	destTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_s2.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Build the source and destination cursors with their transforms.
	srcCursor, _, err := bucket_lookup.BuildEmptyCursor(
		ctx, srcTB.Bus, le, srcTB.StepFactorySet,
		"mm-src", srcTB.Volume.GetID(), srcTransformConf, nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(srcCursor.Release)
	destCursor, _, err := bucket_lookup.BuildEmptyCursor(
		ctx, hostTB.Bus, le, hostTB.StepFactorySet,
		"mm-dest", hostTB.Volume.GetID(), destTransformConf, nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(destCursor.Release)

	// Write the destination under a stage, as the scheduler's World stage
	// does.
	var releaseStage func()
	if err := destCursor.WrapTransactionStore(ctx, func(ctx context.Context, store block.StoreOps) (block.StoreOps, error) {
		staged, release, err := block.OpenStage(ctx, store)
		releaseStage = release
		return staged, err
	}); err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(releaseStage)

	// Small valid manifest: one nested FS directory node referenced by both
	// dist and assets, so the graph is exactly two blocks.
	nodeData, err := (&unixfs_block.FSNode{
		NodeType: unixfs_block.NodeType_NodeType_DIRECTORY,
	}).MarshalVT()
	if err != nil {
		t.Fatal(err.Error())
	}
	dirRef, _, err := srcCursor.PutBlock(ctx, nodeData, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err.Error())
	}
	srcManifest := &bldr_manifest.Manifest{
		Meta: &bldr_manifest.ManifestMeta{
			ManifestId: "mm-test-manifest",
			BuildType:  "production",
			PlatformId: "js",
			Rev:        1,
		},
		Entrypoint:  "entrypoint",
		DistFsRef:   dirRef,
		AssetsFsRef: dirRef,
	}

	// Write the manifest into the source cursor and read back its root ref.
	btx, bcs := srcCursor.BuildTransaction(nil)
	bcs.SetBlock(srcManifest, true)
	manifestRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor.SetRootRef(manifestRef)
	srcRef := srcCursor.GetRef()
	if srcRef.GetRootRef().GetEmpty() {
		t.Fatal("test setup: source ref is empty")
	}

	// Follow the manifest root into a read-only selected cursor, as the
	// scheduler does when executing a manifest.
	srcSel, err := bldr_manifest_world.FollowObjectRefReadOnly(ctx, srcCursor, srcRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(srcSel.Release)

	// Source blocks must be absent from the destination before the call.
	rootRef := srcRef.GetRootRef()
	for _, ref := range []*block.BlockRef{rootRef, dirRef} {
		_, exists, err := destCursor.GetBucket().GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if exists {
			t.Fatal("test setup: destination already holds a source block")
		}
	}

	// Host mux: fallback invoker into the host bus (as buildPluginMux does),
	// plus the host volume ProxyVolume services under the host prefix.
	hostMux := srpc.NewMux(bifrost_rpc.NewInvoker(
		hostTB.Bus,
		bldr_plugin.PluginServerID("bldr-materializer", ""),
		true,
	))
	if err := volume_rpc_server.RegisterProxyVolumeWithPrefix(
		hostMux,
		volume_rpc_server.NewProxyVolume(ctx, hostTB.Volume, false),
		bldr_plugin.HostVolumeServiceIDPrefix,
	); err != nil {
		t.Fatal(err.Error())
	}

	// Expose the host mux on the host bus under the web-worker server identity,
	// exactly as host/web does, and route the plugin's host client through a
	// bus invoker carrying that identity.
	workerServerID := "web-worker/" + bldr_plugin.PluginServerID("bldr-materializer", instanceKey)
	rpcServiceCtrl := bifrost_rpc.NewRpcServiceController(
		controller.NewInfo("test-web-worker-rpc-host", Version, "rpc host for plugin"),
		func(_ context.Context, _ func()) (srpc.Invoker, func(), error) {
			return hostMux, nil, nil
		},
		nil,
		false,
		nil,
		nil,
		regexp.MustCompile("^"+regexp.QuoteMeta(workerServerID)+"$"),
	)
	relRpcServiceCtrl, err := hostTB.Bus.AddController(ctx, rpcServiceCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relRpcServiceCtrl)
	hostClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(
		bifrost_rpc.NewInvoker(hostTB.Bus, workerServerID, true),
	)))

	// Plugin bus: reach host services through the host pipe.
	pluginHostBridge := bifrost_rpc.NewClientController(
		le,
		pluginTB.Bus,
		controller.NewInfo("test-plugin-host-bridge", controller.MustParseVersion("0.0.1"), "plugin bus bridge to host services"),
		hostClient,
		[]string{bldr_plugin.HostServiceIDPrefix},
	)
	relBridge, err := pluginTB.Bus.AddController(ctx, pluginHostBridge, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relBridge)

	// Plugin bus: mount the host volume proxy under the plugin-host volume
	// alias, exactly as the plugin entrypoint does.

	// Build the host volume info and its proxy controller.
	hostVolInfo, err := volume.NewVolumeInfo(
		ctx,
		controller.NewInfo("test-host-volume", controller.MustParseVersion("0.0.1"), "test host volume"),
		hostTB.Volume,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	proxyVolCtrl := volume_rpc_client.NewProxyVolumeControllerWithClient(
		pluginTB.Bus,
		le,
		hostVolInfo,
		[]string{bldr_plugin.PluginVolumeID},
		hostClient,
		bldr_plugin.HostVolumeServiceIDPrefix,
	)
	relProxyVol, err := pluginTB.Bus.AddController(ctx, proxyVolCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relProxyVol)

	// Serve the real Materializer on the plugin bus pipe.
	pluginMux := srpc.NewMux()
	if err := bldr_manifest_materializer.SRPCRegisterMaterializer(
		pluginMux,
		bldr_manifest_materializer.NewMaterializer(le, pluginTB.Bus, pluginTB.StepFactorySet),
	); err != nil {
		t.Fatal(err.Error())
	}
	pluginClient := srpc.NewClient(srpc.NewServerPipe(srpc.NewServer(pluginMux)))

	// Handle RPC lookups and LoadPlugin demands for the materializer.
	ctrl := &Controller{bus: hostTB.Bus, conf: &Config{InstanceKey: instanceKey}}
	demanded := make(chan string, 1)
	released := make(chan struct{})
	relPluginHandler, err := hostTB.Bus.AddHandler(directive.NewFuncHandler(
		func(ctx context.Context, inst directive.Instance) ([]directive.Resolver, error) {
			switch dir := inst.GetDirective().(type) {
			// Route RPC client lookups through the scheduler controller.
			case bifrost_rpc.LookupRpcClient:
				return directive.R(bldr_plugin.ResolveLookupRpcClient(ctx, dir, ctrl))

			// Publish the fixture's plugin client for the materializer demand.
			case bldr_plugin.LoadPlugin:
				if dir.LoadPluginID() != "bldr-materializer" {
					return nil, nil
				}
				return directive.R(directive.NewFuncResolver(
					func(ctx context.Context, handler directive.ResolverHandler) error {
						// Record the demanded instance key and publish the client.
						demanded <- dir.LoadPluginInstanceKey()
						defer close(released)
						_, _ = handler.AddValue(bldr_plugin.NewRunningPlugin(pluginClient))
						handler.MarkIdle(true)
						<-ctx.Done()
						return ctx.Err()
					},
				), nil)
			}
			return nil, nil
		},
	))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(relPluginHandler)

	// Run the materialize copy under a bounded deadline so the RPC completes
	// or fails instead of hanging.
	copyCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	t.Cleanup(cancel)
	copiedRef, _, err := ctrl.materializeManifest(
		copyCtx,
		"bldr-materializer",
		destCursor,
		srcSel,
		2,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// The copy must demand the scheduler's instance and release it on completion.
	select {
	case got := <-demanded:
		if got != instanceKey {
			t.Fatalf("materializer instance key = %q, want %q", got, instanceKey)
		}
	default:
		t.Fatal("copy completed without demanding the materializer plugin")
	}
	select {
	case <-released:
	case <-time.After(2 * time.Second):
		t.Fatal("completed copy retained its materializer plugin demand")
	}

	// The copied root must match the source root hash and sit in the
	// destination bucket.
	if !copiedRef.GetRootRef().EqualsRef(rootRef) {
		t.Fatalf("copied root %s != source root %s",
			copiedRef.GetRootRef().MarshalString(), rootRef.MarshalString())
	}
	if copiedRef.GetBucketId() != "mm-dest" {
		t.Fatalf("copied bucket id = %q, want %q", copiedRef.GetBucketId(), "mm-dest")
	}

	// Both the manifest root and the nested directory node must now exist in
	// the host TB with bytes identical to the source bucket's at-rest bytes.
	for _, ref := range []*struct {
		name string
		ref  *block.BlockRef
	}{{"manifest root", rootRef}, {"directory node", dirRef}} {
		wantData, wantExists, err := srcCursor.GetBucket().GetBlock(ctx, ref.ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !wantExists {
			t.Fatalf("test setup: source bucket missing %s", ref.name)
		}
		gotData, gotExists, err := destCursor.GetBucket().GetBlock(ctx, ref.ref)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !gotExists {
			t.Fatalf("copied %s missing from host destination bucket", ref.name)
		}
		if !bytes.Equal(gotData, wantData) {
			t.Fatalf("copied %s raw bytes != source raw bytes", ref.name)
		}
	}

	// Decode the copied manifest and compare with the source manifest.
	gotCursor, err := destCursor.FollowRef(ctx, copiedRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(gotCursor.Release)
	_, gotBcs := gotCursor.BuildTransaction(nil)
	gotManifest, err := bldr_manifest.UnmarshalManifest(ctx, gotBcs)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !gotManifest.EqualVT(srcManifest) {
		t.Fatalf("decoded manifest = %v, want %v", gotManifest, srcManifest)
	}
}
func TestManifestDemandAccountingIncludesLiveReadsBeforeExecutionExit(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	store := block_mock.NewMockStore(0)
	ref, _, err := block.PutBlock(ctx, store, block_mock.NewExample("live demand"))
	if err != nil {
		t.Fatal(err.Error())
	}
	_, cursor := block.NewTransaction(store, nil, ref, nil)

	// Build the manifest snapshot fixture.
	snapshot := &bldr_manifest.ManifestSnapshot{
		ManifestRef: &bucket.ObjectRef{
			BucketId: "remote",
			RootRef:  ref,
		},
	}
	accounting := newManifestCopyAccounting(snapshot, "remote", "local")
	readCtx, counter := block.WithReadCounter(ctx)
	observation := &manifestDemandObservation{
		accounting: accounting,
		counter:    counter,
	}
	observation.register()
	defer observation.finish()

	// Assert the expected state.
	if _, found, err := cursor.Fetch(readCtx); err != nil || !found {
		t.Fatalf("access-manifest read = found %v, err %v", found, err)
	}
	observation.snapshot()
	callbackStats := accounting.apply(bucket_lookup.ObjectCopyStats{})
	if callbackStats.DemandReadCount == 0 || callbackStats.DemandReadBytes == 0 {
		t.Fatalf("access-manifest demand stats = %#v, want nonzero", callbackStats)
	}

	// Assert the expected state.
	if _, found, err := cursor.Fetch(readCtx); err != nil || !found {
		t.Fatalf("live plugin read = found %v, err %v", found, err)
	}
	liveStats := accounting.apply(bucket_lookup.ObjectCopyStats{})
	if liveStats.DemandReadCount <= callbackStats.DemandReadCount ||
		liveStats.DemandReadBytes <= callbackStats.DemandReadBytes {
		t.Fatalf("live demand stats = %#v, callback stats = %#v", liveStats, callbackStats)
	}
}

func TestSupersededDownloadCannotPublishStatusOrMark(t *testing.T) {
	// Build the test manifest refs and snapshots.
	refA := newTestManifestRef("spacewave-core", "desktop/darwin/arm64", 1, "bucket-a")
	refB := newTestManifestRef("spacewave-core", "desktop/darwin/arm64", 2, "bucket-b")
	snapshotA := &bldr_manifest.ManifestSnapshot{ManifestRef: refA.GetManifestRef()}
	snapshotB := &bldr_manifest.ManifestSnapshot{ManifestRef: refB.GetManifestRef()}
	accountingA := newManifestCopyAccounting(snapshotA, "bucket-a", "local")
	accountingB := newManifestCopyAccounting(snapshotB, "bucket-b", "local")

	// Build the plugin instance with the superseded copy status.
	pi := &pluginInstance{
		manifestCopyStatus: ccontainer.NewCContainer[*manifestCopyStatus](nil),
	}
	pi.manifestCopyAccounting.Store(accountingB)
	pi.manifestCopyStatus.SetValue(&manifestCopyStatus{
		phase:       manifestCopyPhaseCopying,
		manifestRef: snapshotB.GetManifestRef().MarshalString(),
	})

	// A superseded download must not publish status or marks.
	for _, phase := range []manifestCopyPhase{manifestCopyPhaseDone, manifestCopyPhaseFailed} {
		pi.setManifestCopyStatus(
			phase,
			manifestCopyClassImmediate,
			snapshotA,
			accountingA,
			bucket_lookup.ObjectCopyStats{BlocksSeen: 99},
		)
		status := pi.manifestCopyStatus.GetValue()
		if status == nil ||
			status.phase != manifestCopyPhaseCopying ||
			status.manifestRef != snapshotB.GetManifestRef().MarshalString() {
			t.Fatalf("superseded download published %#v over current candidate", status)
		}
		if pi.emitManifestCopyStartupMark(phase, bucket_lookup.ObjectCopyStats{BlocksSeen: 99}, accountingA) {
			t.Fatalf("superseded download emitted %s startup mark", phase)
		}
	}
	if !pi.emitManifestCopyStartupMark(
		manifestCopyPhaseDone,
		bucket_lookup.ObjectCopyStats{},
		accountingB,
	) {
		t.Fatal("current download startup mark was incorrectly gated")
	}
}

func TestDownloadManifestCopiesRemoteDAGAndStoresLocalWorldRef(t *testing.T) {
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

	// Name the buckets used by the fixture.
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

	// Build the manifest object key.
	manifestKey := bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef())
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

	// Declare the world state wrapper.
	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus:           tb.Bus,
			conf:          &Config{},
			objKey:        objKey,
			peerID:        peer.ID("test"),
			worldStateCtr: ccontainer.NewCContainer(wsv),
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

	// Collect the manifests for the controller.
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

	// Exactly one manifest must be collected without errors.
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d, want 1", len(got))
	}

	// The stored local manifest must keep its bucket and content.
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

	// Fetch the manifest store object for the running instance.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host manifest store object")
	}

	// Build the running plugin instance fixture.
	host := &testPluginHost{id: "desktop/darwin/arm64"}
	runningPi := &pluginInstance{
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
	runningArgs := &executePluginArgs{
		manifestSnapshot: &bldr_manifest.ManifestSnapshot{
			ManifestRef: ref.GetManifestRef(),
		},
		pluginHost: host,
	}
	runningPi.executePluginRoutine.SetState(runningArgs)
	if _, _, err := runningPi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj); err != nil {
		t.Fatal(err.Error())
	}
	if runningPi.executePluginRoutine.GetState() != runningArgs {
		t.Fatal("expected running remote manifest to stay active after local copy appears")
	}

	// Build the plugin instance fixture.
	watchPi := &pluginInstance{
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
	if _, _, err := watchPi.processManifestWorldState(ctx, le, &pluginHostSet{
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

func TestDownloadManifestRetriesIncompleteCopyBeforePublication(t *testing.T) {
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

	// Name the buckets used by the fixture.
	const remoteBucketID = "spacewave-release"
	remote := newTestExternalManifestRefWithDistAssets(t, ctx, remoteBucketID, "spacewave-core", "desktop/darwin/arm64", 14)
	sourceStore := &failAfterRootBlockStore{
		StoreOps: remote.store,
		getCalls: &atomic.Int32{},
		failed:   &atomic.Bool{},
	}
	sourceConf, err := bucket.NewConfig(remoteBucketID, 1, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	lookupRel, err := tb.Bus.AddController(ctx, &testSchedulerStaticLookupController{
		bucketID: remoteBucketID,
		handle: &testSchedulerStaticLookupHandle{
			conf:   sourceConf,
			lookup: &testSchedulerStaticLookup{store: sourceStore},
		},
	}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lookupRel()

	// Declare the world state wrapper.
	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus:           tb.Bus,
			conf:          &Config{},
			objKey:        objKey,
			peerID:        peer.ID("test"),
			worldStateCtr: ccontainer.NewCContainer(wsv),
		},
		le:       le,
		pluginID: "spacewave-core",
	}
	snapshot := &bldr_manifest.ManifestSnapshot{
		ManifestRef: remote.ref.GetManifestRef(),
		Manifest:    remote.manifest,
	}

	// The background job retries the attempt that hit the injected failure.
	if err := pi.execDownloadManifest(ctx, snapshot); err != nil {
		t.Fatalf("copy failed: %v", err)
	}
	if !sourceStore.failed.Load() {
		t.Fatal("copy finished without reaching the injected descendant failure")
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
		t.Fatalf("manifest errors after retry = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count after retry = %d, want 1", len(got))
	}
	localRef := got[0].ManifestRef
	if err := bldr_manifest_world.AccessManifest(
		ctx,
		le,
		ws.AccessWorldState,
		localRef,
		func(
			ctx context.Context,
			_ *bucket_lookup.Cursor,
			_ *block.Cursor,
			manifest *bldr_manifest.Manifest,
			distFS *unixfs.FSHandle,
			assetsFS *unixfs.FSHandle,
		) error {
			if _, _, err := distFS.LookupPath(ctx, manifest.GetEntrypoint()); err != nil {
				return err
			}
			if _, _, err := assetsFS.LookupPath(ctx, "asset.txt"); err != nil {
				return err
			}
			return nil
		},
	); err != nil {
		t.Fatalf("incomplete local manifest DAG after retry: %v", err)
	}
}

func TestDownloadManifestCopiesExternalVolumeDAGAndCachesSourceReads(t *testing.T) {
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
	const objKey = "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}

	// Name the buckets used by the fixture.
	const remoteBucketID = "spacewave-release"
	remote := newTestExternalManifestRefWithDistAssets(t, ctx, remoteBucketID, "spacewave-core", "desktop/darwin/arm64", 14)

	// Build the writeback cache over the remote store.
	cacheKey, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	cache := block_store_kvtx.NewKVTxBlock(cacheKey, store_kvtx_inmem.NewStore(), 0)
	networkGets := &atomic.Uint32{}
	sourceStore := &writebackLookupBlockStore{
		StoreOps:    remote.store,
		cache:       cache,
		networkGets: networkGets,
	}

	// Register the remote bucket as a static lookup source.
	sourceConf, err := bucket.NewConfig(remoteBucketID, 1, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	lookupRel, err := tb.Bus.AddController(ctx, &testSchedulerStaticLookupController{
		bucketID: remoteBucketID,
		handle: &testSchedulerStaticLookupHandle{
			conf:   sourceConf,
			lookup: &testSchedulerStaticLookup{store: sourceStore},
		},
	}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lookupRel()

	// Declare the fixture state.
	var worldBucketID string
	if err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		worldBucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Declare the world state wrapper.
	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus:           tb.Bus,
			conf:          &Config{},
			objKey:        objKey,
			peerID:        peer.ID("test"),
			worldStateCtr: ccontainer.NewCContainer(wsv),
		},
		le:       le,
		pluginID: "spacewave-core",
	}
	snapshot := &bldr_manifest.ManifestSnapshot{
		ManifestRef: remote.ref.GetManifestRef(),
		Manifest:    remote.manifest,
	}

	// Declare the fixture state.
	var firstRoot *block.BlockRef
	for i := range 2 {
		if err := pi.execDownloadManifest(ctx, snapshot); err != nil {
			t.Fatalf("copy attempt %d: %v", i+1, err)
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
			t.Fatalf("manifest errors after copy attempt %d = %v", i+1, errs)
		}
		if len(got) != 1 {
			t.Fatalf("manifest count after copy attempt %d = %d, want 1", i+1, len(got))
		}
		localRef := got[0].ManifestRef
		if localRef.GetBucketId() != worldBucketID {
			t.Fatalf("manifest bucket after copy attempt %d = %q, want %q", i+1, localRef.GetBucketId(), worldBucketID)
		}
		if !localRef.GetRootRef().EqualVT(remote.ref.GetManifestRef().GetRootRef()) {
			t.Fatalf("manifest root after copy attempt %d changed", i+1)
		}
		if !got[0].Manifest.GetMeta().EqualVT(remote.manifest.GetMeta()) {
			t.Fatalf("manifest metadata after copy attempt %d changed", i+1)
		}
		if firstRoot == nil {
			firstRoot = localRef.GetRootRef().CloneVT()
		} else if !firstRoot.EqualVT(localRef.GetRootRef()) {
			t.Fatal("local manifest root changed across cache-only copy")
		}
	}
	if got := networkGets.Load(); got == 0 {
		t.Fatal("external manifest copy did not demand any source blocks")
	}
	firstGets := networkGets.Load()
	if err := pi.execDownloadManifest(ctx, snapshot); err != nil {
		t.Fatal(err.Error())
	}
	if got := networkGets.Load(); got != firstGets {
		t.Fatalf("source network fetches after cache-only copy = %d, want %d", got, firstGets)
	}
}

func TestDownloadManifestCopiesSeveralRemoteDAGsOutsideWorldAccess(t *testing.T) {
	// Bound the test with a timeout.
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
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

	// Wrap the World state with access counting.
	ws := &accessCountingWorldState{WorldState: baseWS}
	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus: tb.Bus,
			conf: &Config{
				FetchConcurrency: 1,
			},
			objKey:        objKey,
			peerID:        peer.ID("test"),
			worldStateCtr: ccontainer.NewCContainer(wsv),
		},
		le:       le,
		pluginID: "spacewave-core",
	}

	// Queue three requests against one aggregate copy allowance of one.
	type manifestCopyRequest struct {
		ctx        context.Context
		manifestID string
		snapshot   *bldr_manifest.ManifestSnapshot
		errCh      chan error
		cancel     context.CancelFunc
	}
	requests := make([]*manifestCopyRequest, 0, 3)
	observed := make(chan manifestCopyObservation, 3)
	releaseCopy := make(chan struct{})
	for i := range 3 {
		suffix := string(rune('a' + i))
		remoteBucketID := "remote-manifest-bucket-" + suffix
		remote := newTestExternalManifestRefWithDistAssets(
			t,
			ctx,
			remoteBucketID,
			"spacewave-core-"+suffix,
			"desktop/darwin/arm64",
			uint64(20+i),
		)
		sourceStore := &blockingLookupBlockStore{
			StoreOps:     remote.store,
			activeAccess: &ws.active,
			observed:     observed,
			release:      releaseCopy,
			once:         &sync.Once{},
			manifestID:   remote.ref.GetMeta().GetManifestId(),
		}
		sourceConf, err := bucket.NewConfig(remoteBucketID, 1, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		lookupRel, err := tb.Bus.AddController(ctx, &testSchedulerStaticLookupController{
			bucketID: remoteBucketID,
			handle: &testSchedulerStaticLookupHandle{
				conf: sourceConf,
				lookup: &testSchedulerStaticLookup{
					store: sourceStore,
				},
			},
		}, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer lookupRel()

		reqCtx, reqCancel := context.WithCancel(ctx)
		defer reqCancel()
		requests = append(requests, &manifestCopyRequest{
			ctx:        reqCtx,
			manifestID: remote.ref.GetMeta().GetManifestId(),
			snapshot: &bldr_manifest.ManifestSnapshot{
				ManifestRef: remote.ref.GetManifestRef(),
				Manifest:    remote.manifest,
			},
			errCh:  make(chan error, 1),
			cancel: reqCancel,
		})
	}
	for _, req := range requests {
		go func() {
			req.errCh <- pi.execDownloadManifest(req.ctx, req.snapshot)
		}()
	}

	// The first admitted copy enters traversal alone. The queued requests
	// must not start reading source blocks.
	var activeReq *manifestCopyRequest
	var activeID string
	select {
	case obs := <-observed:
		if obs.active != 0 {
			t.Fatalf("source block read for %s ran with %d active world access(es)", obs.manifestID, obs.active)
		}
		activeID = obs.manifestID
		for _, req := range requests {
			if req.manifestID == obs.manifestID {
				activeReq = req
			}
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for the first source observation: %v", ctx.Err())
	}
	if activeReq == nil {
		t.Fatalf("observed unknown manifest %s", activeID)
	}
	select {
	case obs := <-observed:
		t.Fatalf("queued request %s entered traversal while %s held the copy allowance", obs.manifestID, activeReq.manifestID)
	default:
	}

	// Foreground World access must complete while the active copy holds the
	// allowance: the copy holds no World access of its own.
	fgCtx, fgCancel := context.WithTimeout(ctx, 2*time.Second)
	defer fgCancel()
	var worldBucketID string
	if err := ws.AccessWorldState(fgCtx, nil, func(cursor *bucket_lookup.Cursor) error {
		worldBucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatalf("foreground world access failed while a copy held admission: %v", err)
	}
	if worldBucketID == "" {
		t.Fatal("foreground world access returned an empty bucket id")
	}

	// Cancel one queued request while it waits for admission. It must exit
	// without entering traversal or publishing.
	var cancelReq, queuedReq *manifestCopyRequest
	for _, req := range requests {
		if req == activeReq {
			continue
		}
		if cancelReq == nil {
			cancelReq = req
		} else {
			queuedReq = req
		}
	}
	cancelReq.cancel()
	select {
	case err := <-cancelReq.errCh:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled queued copy failed with %v, want context.Canceled", err)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for the canceled queued copy: %v", ctx.Err())
	}
	got, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		baseWS,
		cancelReq.manifestID,
		[]string{"desktop/darwin/arm64"},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors for canceled copy %s = %v", cancelReq.manifestID, errs)
	}
	if len(got) != 0 {
		t.Fatalf("canceled queued copy published %d manifests, want 0", len(got))
	}

	// Release the active copy. It completes and publishes its copied root.
	close(releaseCopy)
	select {
	case err := <-activeReq.errCh:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for the active manifest copy: %v", ctx.Err())
	}

	// The remaining queued request proceeds only after the allowance is
	// released: its first observation must record the release channel already
	// closed.
	select {
	case obs := <-observed:
		if obs.manifestID != queuedReq.manifestID {
			t.Fatalf("observed source read for %s, want %s", obs.manifestID, queuedReq.manifestID)
		}
		if obs.active != 0 {
			t.Fatalf("source block read for %s ran with %d active world access(es)", obs.manifestID, obs.active)
		}
		if obs.copyBlocked {
			t.Fatalf("queued request %s entered traversal before the copy allowance released", obs.manifestID)
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for the queued copy to proceed: %v", ctx.Err())
	}
	select {
	case err := <-queuedReq.errCh:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for the queued manifest copy: %v", ctx.Err())
	}

	// Iterate the fixture cases.
	for _, req := range []*manifestCopyRequest{activeReq, queuedReq} {
		got, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
			ctx,
			baseWS,
			req.manifestID,
			[]string{"desktop/darwin/arm64"},
			objKey,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		if len(errs) != 0 {
			t.Fatalf("manifest errors for %s = %v", req.manifestID, errs)
		}
		if len(got) != 1 {
			t.Fatalf("manifest count for %s = %d, want 1", req.manifestID, len(got))
		}
	}
}

func TestDownloadManifestYieldsColdStartCopyUntilStartupGroupReady(t *testing.T) {
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

	// Name the buckets used by the fixture.
	const remoteBucketID = "remote-manifest-bucket"
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  remoteBucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}
	ref := newTestStoredManifestRefWithDistInBucket(t, ctx, tb, remoteBucketID, "spacewave-core", "desktop/darwin/arm64", 2)
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
	snapshot := &bldr_manifest.ManifestSnapshot{
		ManifestRef: ref.GetManifestRef(),
		Manifest:    remoteManifest,
	}

	// Declare the world state wrapper.
	var wsv world.WorldState = ws
	gate := newTestManifestCopyGate(false)
	pi := &pluginInstance{
		c: &Controller{
			bus:                 tb.Bus,
			conf:                &Config{},
			objKey:              objKey,
			peerID:              peer.ID("test"),
			worldStateCtr:       ccontainer.NewCContainer(wsv),
			manifestCopyGateCtr: ccontainer.NewCContainer[ManifestCopyGate](gate),
		},
		le:                      le,
		pluginID:                "spacewave-core",
		runningPluginCtr:        ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		manifestCopyStatus:      ccontainer.NewCContainer[*manifestCopyStatus](nil),
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}
	pi.executePluginRoutine.SetState(&executePluginArgs{
		manifestSnapshot: snapshot,
		pluginHost:       &testPluginHost{id: "desktop/darwin/arm64"},
	})

	// The copy publishes only for the routine's currently selected manifest.
	// Establish the selection this test drives directly.
	pi.downloadManifestRoutine.SetState(snapshot)

	// Bound the test with a timeout.
	execCtx, execCancel := context.WithCancel(ctx)
	defer execCancel()
	copyErrCh := make(chan error, 1)
	go func() {
		copyErrCh <- pi.execDownloadManifest(execCtx, snapshot)
	}()

	// Bound the test with a timeout.
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	status, err := pi.manifestCopyStatus.WaitValueWithValidator(waitCtx, func(status *manifestCopyStatus) (bool, error) {
		return status != nil && status.phase == manifestCopyPhaseWaitingForStartupGroup, nil
	}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if status.class != manifestCopyClassAfterStartupGroupReady {
		t.Fatalf("copy class = %q, want %q", status.class, manifestCopyClassAfterStartupGroupReady)
	}
	select {
	case err := <-copyErrCh:
		t.Fatalf("copy completed before startup group readiness: %v", err)
	default:
	}

	// Collect the manifests for the controller.
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
		t.Fatalf("manifest count before startup group readiness = %d, want 0", len(got))
	}

	// Open the readiness gate.
	gate.readyCtr.SetValue(true)
	select {
	case err := <-copyErrCh:
		if err != nil {
			t.Fatal(err.Error())
		}
	case <-waitCtx.Done():
		t.Fatal(waitCtx.Err().Error())
	}
	status = pi.manifestCopyStatus.GetValue()
	if status == nil || status.phase != manifestCopyPhaseDone {
		t.Fatalf("copy status = %#v, want done", status)
	}
	if status.stats.BlocksSeen == 0 ||
		status.stats.BlocksCopied != status.stats.BlocksWritten+status.stats.BlocksExisting ||
		status.stats.LogicalSourceBytes == 0 {
		t.Fatalf("copy accounting = %#v, want complete logical copy totals", status.stats)
	}
	if status.sourceBucketID != remoteBucketID ||
		status.sourceIdentity != manifestCopyIdentityExternal ||
		status.destinationBucketID == "" ||
		status.destinationIdentity != manifestCopyIdentityLocal {
		t.Fatalf("copy identity = %#v, want external source and local destination", status)
	}
	if status.stats.DestinationDurableBytesKnown &&
		status.stats.DestinationDurableBytes != status.stats.LogicalSourceBytes {
		t.Fatalf("durable bytes = %#v, want logical source bytes", status.stats)
	}
}

func TestDownloadManifestCopiesTransformedRemoteDAGAndStoresLocalWorldRef(t *testing.T) {
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

	// Name the buckets used by the fixture.
	const remoteBucketID = "remote-transformed-manifest-bucket"
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  remoteBucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}
	transformConf, err := block_transform.NewConfig([]config.Config{&transform_gzip.Config{}})
	if err != nil {
		t.Fatal(err.Error())
	}
	ref := newTestStoredManifestRefWithDistInBucketAndTransform(
		t,
		ctx,
		tb,
		remoteBucketID,
		"spacewave-core",
		"desktop/darwin/arm64",
		2,
		transformConf,
	)
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

	// Declare the fixture state.
	var remoteManifest *bldr_manifest.Manifest
	if err := bldr_manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, ref.GetManifestRef(), func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
		// Record the served remote manifest.
		remoteManifest = manifest.CloneVT()
		file, _, err := distFS.LookupPath(ctx, manifest.GetEntrypoint())
		if err != nil {
			return err
		}
		_, err = unixfs.ReadFile(ctx, file)
		return err
	}); err != nil {
		t.Fatal(err.Error())
	}
	if remoteManifest == nil {
		t.Fatal("expected remote manifest to be decoded")
	}

	// Declare the world state wrapper.
	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus:           tb.Bus,
			conf:          &Config{},
			objKey:        objKey,
			peerID:        peer.ID("test"),
			worldStateCtr: ccontainer.NewCContainer(wsv),
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

	// Collect the manifests for the controller.
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

	// Exactly one manifest must be collected without errors.
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d, want 1", len(got))
	}

	// The stored local manifest must keep its bucket and content.
	localRef := got[0].ManifestRef
	if localRef.GetBucketId() != worldBucketID {
		t.Fatalf("manifest bucket = %q, want local world bucket %q", localRef.GetBucketId(), worldBucketID)
	}
	if !localRef.GetRootRef().EqualVT(ref.GetManifestRef().GetRootRef()) {
		t.Fatal("local manifest root ref changed")
	}
	if !localRef.GetTransformConf().EqualVT(transformConf) {
		t.Fatal("local manifest transform config changed")
	}
	if err := bldr_manifest_world.AccessManifest(ctx, le, ws.AccessWorldState, localRef, func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
		// Look up the plugin file in the filesystem.
		file, _, err := distFS.LookupPath(ctx, manifest.GetEntrypoint())
		if err != nil {
			return err
		}
		data, err := unixfs.ReadFile(ctx, file)
		if err != nil {
			return err
		}
		if string(data) != "console.log('startup')\n" {
			t.Fatalf("entrypoint bytes = %q", data)
		}
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
}

func TestDownloadManifestRejectsMissingSnapshotMetadataBeforeStore(t *testing.T) {
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

	// Name the buckets used by the fixture.
	const remoteBucketID = "remote-manifest-bucket"
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  remoteBucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}
	ref := newTestStoredManifestRefWithDistInBucket(t, ctx, tb, remoteBucketID, "spacewave-core", "desktop/darwin/arm64", 2)

	// Declare the world state wrapper.
	var wsv world.WorldState = ws
	pi := &pluginInstance{
		c: &Controller{
			bus:           tb.Bus,
			conf:          &Config{},
			objKey:        objKey,
			peerID:        peer.ID("test"),
			worldStateCtr: ccontainer.NewCContainer(wsv),
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

	// Collect the manifests for the controller.
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
