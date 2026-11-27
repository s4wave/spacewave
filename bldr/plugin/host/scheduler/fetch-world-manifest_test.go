package plugin_host_scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	configset "github.com/aperturerobotics/controllerbus/controller/configset"
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
	plugin_host_controller "github.com/s4wave/spacewave/bldr/plugin/host/controller"
	block "github.com/s4wave/spacewave/db/block"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_controller "github.com/s4wave/spacewave/db/block/store/controller"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	bucket "github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/dex"
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

func TestFetchManifestValueStorerRepairsMissingManifestLink(t *testing.T) {
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
	ref := newTestStoredManifestRef(t, ctx, tb, "spacewave-core", "desktop/darwin/arm64", 1)
	manifestKey := bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef())
	if _, _, err := bldr_manifest_world.SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
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
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 0 {
		t.Fatalf("expected orphaned manifest to be unreachable, got %d", len(got))
	}

	// Declare the world state wrapper.
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

	// Collect the manifests for the controller.
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

	// Build the manifest object key.
	manifestKey := bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef())
	if err := bldr_manifest_world.ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{objKey}, ref); err != nil {
		t.Fatal(err.Error())
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host manifest store object")
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
		manifestCopyStatus:      ccontainer.NewCContainer[*manifestCopyStatus](nil),
		downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
		executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
	}

	// Process the manifest World state.
	waitForChanges, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
		pluginHosts: []bldr_plugin_host.PluginHost{host},
	}, ws, obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !waitForChanges {
		t.Fatal("expected world manifest watch to continue")
	}

	// Check the download manifest routine state.
	downloadState := pi.downloadManifestRoutine.GetState()
	if downloadState == nil {
		t.Fatal("expected remote manifest to schedule background DAG copy")
	}
	if !downloadState.GetManifestRef().EqualVT(ref.GetManifestRef()) {
		t.Fatal("download manifest ref changed")
	}

	// Check the execute plugin routine state.
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
	copyStatus := pi.manifestCopyStatus.GetValue()
	if copyStatus == nil ||
		copyStatus.phase != manifestCopyPhaseSelected ||
		copyStatus.sourceBucketID != remoteBucketID ||
		copyStatus.destinationBucketID == "" ||
		copyStatus.sourceIdentity != manifestCopyIdentityExternal ||
		copyStatus.destinationIdentity != manifestCopyIdentityLocal {
		t.Fatalf("selected manifest accounting = %#v, want external source and local destination", copyStatus)
	}
	if execState.manifestSnapshot.GetManifest() == nil ||
		!execState.manifestSnapshot.GetManifest().GetMeta().EqualVT(ref.GetMeta()) {
		t.Fatal("execute manifest metadata changed")
	}
}

func TestProcessManifestWorldStateSuppressesNoCopyBucketWhileDynamicManifestCopies(t *testing.T) {
	// Bound the test with a timeout.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the World testbed.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Build an empty cursor for the World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)

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
		platformID      = "desktop/darwin/arm64"
		suppressedID    = "embedded-plugin"
		dynamicID       = "dynamic-plugin"
	)
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, ws, objKey); err != nil {
		t.Fatal(err.Error())
	}
	for _, bucketID := range []string{noCopyBucketID, dynamicBucketID} {
		if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
			Id:  bucketID,
			Rev: 1,
		}); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Store a test manifest reference.
	suppressedRef := newTestStoredManifestRefWithDistInBucket(
		t,
		ctx,
		tb,
		noCopyBucketID,
		suppressedID,
		platformID,
		2,
	)
	dynamicRef := newTestStoredManifestRefWithDistInBucket(
		t,
		ctx,
		tb,
		dynamicBucketID,
		dynamicID,
		platformID,
		2,
	)
	for _, ref := range []*bldr_manifest.ManifestRef{suppressedRef, dynamicRef} {
		manifestKey := bldr_manifest.NewManifestArtifactKey(ref.GetManifestRef())
		if err := bldr_manifest_world.ExStoreManifestOp(
			ctx,
			ws,
			peer.ID("test"),
			manifestKey,
			[]string{objKey},
			ref,
		); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatal("expected plugin host manifest store object")
	}
	var worldBucketID string
	if err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		worldBucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Declare the world state wrapper.
	var wsv world.WorldState = ws
	c := &Controller{
		bus: tb.Bus,
		conf: &Config{
			NoCopyBucketIds: []string{noCopyBucketID},
		},
		objKey:        objKey,
		peerID:        peer.ID("test"),
		worldStateCtr: ccontainer.NewCContainer(wsv),
	}
	host := &testPluginHost{id: platformID}
	newInstance := func(pluginID string) *pluginInstance {
		return &pluginInstance{
			c:                       c,
			le:                      le,
			pluginID:                pluginID,
			instanceKey:             pluginID,
			runningPluginCtr:        ccontainer.NewCContainer(bldr_plugin.NewRunningPlugin(nil)),
			manifestCopyStatus:      ccontainer.NewCContainer[*manifestCopyStatus](nil),
			downloadManifestRoutine: routine.NewStateRoutineContainerWithLoggerVT[*bldr_manifest.ManifestSnapshot](le),
			executePluginRoutine:    routine.NewStateRoutineContainerWithLogger(executePluginArgsEqual, le),
		}
	}
	process := func(pi *pluginInstance) {
		t.Helper()
		wait, _, err := pi.processManifestWorldState(ctx, le, &pluginHostSet{
			pluginHosts: []bldr_plugin_host.PluginHost{host},
		}, ws, obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		if !wait {
			t.Fatal("expected world manifest watch to continue")
		}
	}

	// Create the plugin instance fixture.
	suppressed := newInstance(suppressedID)
	process(suppressed)
	if suppressed.downloadManifestRoutine.GetState() != nil {
		t.Fatal("configured no-copy manifest set download state")
	}
	suppressedExec := suppressed.executePluginRoutine.GetState()
	if suppressedExec == nil || suppressedExec.manifestSnapshot == nil {
		t.Fatal("configured no-copy manifest was not executable")
	}
	status := suppressed.manifestCopyStatus.GetValue()
	if status == nil ||
		status.phase != manifestCopyPhaseSuppressed ||
		status.class != manifestCopyClassSuppressed ||
		status.sourceBucketID != noCopyBucketID ||
		status.destinationBucketID != worldBucketID {
		t.Fatalf("suppressed copy status = %#v", status)
	}
	if err := suppressed.execDownloadManifest(ctx, suppressedExec.manifestSnapshot); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin instance fixture.
	dynamic := newInstance(dynamicID)
	process(dynamic)
	dynamicDownload := dynamic.downloadManifestRoutine.GetState()
	if dynamicDownload == nil {
		t.Fatal("dynamic manifest did not set download state")
	}
	if dynamicDownload.GetManifestRef().GetBucketId() != dynamicBucketID {
		t.Fatalf("dynamic source bucket = %q, want %q", dynamicDownload.GetManifestRef().GetBucketId(), dynamicBucketID)
	}
	if err := dynamic.execDownloadManifest(ctx, dynamicDownload); err != nil {
		t.Fatal(err.Error())
	}
	dynamicStatus := dynamic.manifestCopyStatus.GetValue()
	if dynamicStatus == nil || dynamicStatus.phase != manifestCopyPhaseDone {
		t.Fatalf("dynamic copy status = %#v, want done", dynamicStatus)
	}

	// Collect the manifests for the controller.
	suppressedManifests, suppressedErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		suppressedID,
		[]string{platformID},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(suppressedErrs) != 0 {
		t.Fatalf("suppressed manifest errors = %v", suppressedErrs)
	}
	if len(suppressedManifests) != 1 {
		t.Fatalf("suppressed manifest count = %d, want 1", len(suppressedManifests))
	}
	if got := suppressedManifests[0].ManifestRef.GetBucketId(); got != noCopyBucketID {
		t.Fatalf("suppressed manifest bucket = %q, want authoritative external bucket %q", got, noCopyBucketID)
	}

	// Collect the manifests for the controller.
	dynamicManifests, dynamicErrs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		ws,
		dynamicID,
		[]string{platformID},
		objKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(dynamicErrs) != 0 {
		t.Fatalf("dynamic manifest errors = %v", dynamicErrs)
	}
	if len(dynamicManifests) != 1 {
		t.Fatalf("dynamic manifest count = %d, want 1", len(dynamicManifests))
	}
	if got := dynamicManifests[0].ManifestRef.GetBucketId(); got != worldBucketID {
		t.Fatalf("dynamic manifest bucket = %q, want local world bucket %q", got, worldBucketID)
	}
}

func TestCollectStartupManifestEligibilityDemandsExternalRefAndUsesWriteback(t *testing.T) {
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
	const lookupBucketID = "startup-demand-release-bucket"
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

	// Create the startup demand lookup observer.
	lookupObserver := &startupDemandLookupObserver{waiting: make(chan struct{}, 1)}
	observerRel, err := tb.Bus.AddController(ctx, lookupObserver, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer observerRel()

	// Build the external manifest ref fixture.
	remote := newTestExternalManifestRefWithDistAssets(
		t,
		ctx,
		lookupBucketID,
		"spacewave-core",
		"desktop/darwin/arm64",
		14,
	)
	cacheKey, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	cacheStore := block_store_kvtx.NewKVTxBlock(cacheKey, store_kvtx_inmem.NewStore(), 0)
	networkGets := &atomic.Uint32{}
	provider := block_store.NewStore(
		"test/startup-demand-provider",
		&writebackLookupBlockStore{
			StoreOps:    remote.store,
			cache:       cacheStore,
			networkGets: networkGets,
		},
	)
	storeCtrl := block_store_controller.NewController(
		le,
		controller.NewInfo("test/startup-demand-provider", controller.MustParseVersion("0.0.1"), ""),
		block_store_controller.NewBlockStoreBuilder(provider),
		nil,
		true,
		[]string{lookupBucketID},
		true,
		false,
	)

	// Build the manifest object key.
	manifestKey := bldr_manifest.NewManifestArtifactKey(remote.ref.GetManifestRef())
	if err := bldr_manifest_world.ExStoreManifestOp(
		ctx,
		ws,
		peer.ID("test"),
		manifestKey,
		[]string{objKey},
		remote.ref,
	); err != nil {
		t.Fatal(err.Error())
	}

	// Define the collect helper.
	collect := func() ([]*bldr_manifest_world.StartupManifestCandidateEligibility, error) {
		collectCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		return bldr_manifest_world.CollectStartupManifestEligibilityForManifestID(
			collectCtx,
			ws,
			"spacewave-core",
			[]string{"desktop/darwin/arm64"},
			objKey,
		)
	}

	// Declare the collect result type.
	type collectResult struct {
		candidates []*bldr_manifest_world.StartupManifestCandidateEligibility
		err        error
	}
	collectDone := make(chan collectResult, 1)
	go func() {
		candidates, err := collect()
		collectDone <- collectResult{candidates: candidates, err: err}
	}()

	// Wait for the expected outcome.
	select {
	case <-lookupObserver.waiting:
	case result := <-collectDone:
		t.Fatalf("startup eligibility completed before lookup provider readiness: %v", result.err)
	case <-time.After(5 * time.Second):
		t.Fatal("startup eligibility did not reach remote block lookup")
	}
	select {
	case result := <-collectDone:
		t.Fatalf("startup eligibility completed before provider registration: %v", result.err)
	default:
	}

	// Start the manifest store controller.
	storeRel, err := tb.Bus.AddController(ctx, storeCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer storeRel()

	// Wait for the collection to finish.
	result := <-collectDone
	if result.err != nil {
		t.Fatal(result.err.Error())
	}
	first := result.candidates

	// Assert the expected state.
	if len(first) != 1 || first[0].Eligibility != bldr_manifest_world.StartupManifestEligibilityEligible {
		t.Fatalf("first startup candidates = %s", bldr_manifest_world.SummarizeStartupManifestEligibility(first, -1))
	}

	// Assert the expected state.
	if !first[0].ManifestRef.EqualVT(remote.ref.GetManifestRef()) ||
		!first[0].Manifest.GetMeta().EqualVT(remote.ref.GetMeta()) {
		t.Fatal("first startup candidate changed external ref or metadata")
	}
	if selected := bldr_manifest_world.SelectableStartupManifests(first); len(selected) != 1 {
		t.Fatalf("selected startup manifests = %d, want 1", len(selected))
	}
	if got := networkGets.Load(); got != 1 {
		t.Fatalf("network block fetches after first startup read = %d, want 1", got)
	}

	// Collect the manifests a second time.
	second, err := collect()
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(second) != 1 || second[0].Eligibility != bldr_manifest_world.StartupManifestEligibilityEligible {
		t.Fatalf("second startup candidates = %s", bldr_manifest_world.SummarizeStartupManifestEligibility(second, -1))
	}
	if got := networkGets.Load(); got != 1 {
		t.Fatalf("network block fetches after cached startup read = %d, want 1", got)
	}
}

func TestExecPluginReadsExternalManifestViaLookupBlockFromNetwork(t *testing.T) {
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

	// Build the external manifest ref fixture.
	remote := newTestExternalManifestRefWithDistAssets(t, ctx, lookupBucketID, "spacewave-core", "desktop/darwin/arm64", 3)
	remoteStore := block_store.NewStore("test/release-world-cdn", remote.store)
	storeCtrl := block_store_controller.NewController(
		le,
		controller.NewInfo("test/release-world-cdn-store", controller.MustParseVersion("0.0.1"), ""),
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

	// Name the fixture constants.
	const refKey = "plugin-host/ref/release-world-cdn"
	storeTestManifestRefObject(t, ctx, ws, refKey, remote.ref)
	if err := ws.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(objKey, refKey, "spacewave-core")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect the manifests for the controller.
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

	// Create the plugin host fixture.
	host := &releaseCDNRuntimePluginHost{
		id: "desktop/darwin/arm64",
	}
	hostCtrl := plugin_host_controller.NewController(
		le,
		tb.Bus,
		controller.NewInfo("test/plugin-host", controller.MustParseVersion("0.0.1"), ""),
		host,
	)
	hostRel, err := tb.Bus.AddController(ctx, hostCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer hostRel()

	// Declare the world state wrapper.
	var wsv world.WorldState = ws

	// Build the plugin instance fixture.
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
		},
		le:               le,
		pluginID:         "spacewave-core",
		runningPluginCtr: ccontainer.NewCContainer[bldr_plugin.RunningPlugin](nil),
		pluginLoadStateCtr: ccontainer.NewCContainer(
			bldr_plugin.NewPluginLoadState(
				nil,
				bldr_plugin.InitialCapabilityRegistrationPending,
			),
		),
	}

	// Execute the plugin with the remote manifest.
	if err := pi.execPlugin(ctx, &executePluginArgs{
		manifestSnapshot: &bldr_manifest.ManifestSnapshot{
			ManifestRef: remote.ref.GetManifestRef(),
			Manifest:    remote.manifest,
		},
		pluginHost: host,
	}); err != nil {
		t.Fatal(err.Error())
	}

	// The demand execution must read the remote blocks.
	if remote.store.gets.Load() == 0 {
		t.Fatal("expected demand execution to invoke LookupBlockFromNetwork")
	}
	accounting := pi.manifestCopyAccounting.Load()
	stats := accounting.apply(bucket_lookup.ObjectCopyStats{})
	if stats.DemandReadCount == 0 || stats.DemandReadBytes == 0 {
		t.Fatalf("expected nonzero demand accounting, got %#v", stats)
	}
	if string(host.distData) != "console.log('release cdn')\n" {
		t.Fatalf("dist entrypoint bytes = %q", host.distData)
	}
	if string(host.assetsData) != "release asset\n" {
		t.Fatalf("asset bytes = %q", host.assetsData)
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
	// Mark the helper.
	t.Helper()

	// Build the manifest meta fixture.
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

	// Write the manifest block in a transaction.
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
	// Mark the helper.
	t.Helper()

	// Build the manifest meta fixture.
	meta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_RELEASE, platformID, rev)
	var ref *bucket.ObjectRef
	err := ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		// Write the manifest block in a transaction.
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

	// Build the manifest object key.
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
	return newTestStoredManifestRefWithDistInBucketAndTransform(t, ctx, tb, bucketID, manifestID, platformID, rev, nil)
}

func newTestStoredManifestRefWithDistInBucketAndTransform(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	bucketID,
	manifestID,
	platformID string,
	rev uint64,
	transformConf *block_transform.Config,
) *bldr_manifest.ManifestRef {
	// Mark the helper.
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

	// Build the manifest meta fixture.
	meta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_RELEASE, platformID, rev)
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		bucketID,
		tb.Volume.GetID(),
		transformConf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	// Write the manifest block in a transaction.
	btx, bcs := oc.BuildTransaction(nil)
	if err := bldr_manifest.CreateManifestWithBilly(ctx, bcs, bldr_manifest.NewManifest(meta, entrypoint), distFS, nil, timestamppb.Now()); err != nil {
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

type failAfterRootBlockStore struct {
	block.StoreOps
	getCalls *atomic.Int32
	failed   *atomic.Bool
}

func (s *failAfterRootBlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &failAfterRootBlockStore{
		StoreOps: store,
		getCalls: s.getCalls,
		failed:   s.failed,
	}, release, nil
}

func (s *failAfterRootBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if s.getCalls.Add(1) == 2 && s.failed.CompareAndSwap(false, true) {
		return nil, false, errors.New("injected descendant failure")
	}
	return s.StoreOps.GetBlock(ctx, ref)
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
	// Mark the helper.
	t.Helper()

	// Name the fixture constants.
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

	// Create the in-memory assets filesystem.
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

	// Build the kv key for the store.
	kvk, err := store_kvkey.NewKVKey(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	ops := block_store_kvtx.NewKVTxBlock(kvk, store_kvtx_inmem.NewStore(), 0)
	store := &countingBlockStore{store: ops, gets: &atomic.Uint32{}}

	// Create the manifest block in the store.
	meta := bldr_manifest.NewManifestMeta(manifestID, bldr_manifest.BuildType_RELEASE, platformID, rev)
	btx, bcs := block.NewTransaction(store, nil, nil, nil)
	manifest := bldr_manifest.NewManifest(meta, entrypoint)
	if err := bldr_manifest.CreateManifestWithBilly(ctx, bcs, manifest, distFS, assetsFS, timestamppb.Now()); err != nil {
		t.Fatal(err.Error())
	}
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Return the external manifest ref fixture.
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

type accessCountingWorldState struct {
	world.WorldState
	active atomic.Int32
	total  atomic.Int32
}

func (s *accessCountingWorldState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	s.total.Add(1)
	s.active.Add(1)
	defer s.active.Add(-1)
	return s.WorldState.AccessWorldState(ctx, ref, cb)
}

type manifestCopyObservation struct {
	manifestID string
	active     int32
	// copyBlocked reports whether the read still waits on release,
	// observed with a nonblocking select at emission time.
	copyBlocked bool
}

type blockingLookupBlockStore struct {
	block.StoreOps
	activeAccess *atomic.Int32
	observed     chan<- manifestCopyObservation
	release      <-chan struct{}
	once         *sync.Once
	manifestID   string
}

func (s *blockingLookupBlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// Open a read operation on the store.
	store, release, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	once := s.once
	if once == nil {
		once = &sync.Once{}
	}
	return &blockingLookupBlockStore{
		StoreOps:     store,
		activeAccess: s.activeAccess,
		observed:     s.observed,
		release:      s.release,
		once:         once,
		manifestID:   s.manifestID,
	}, release, nil
}

func (s *blockingLookupBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	s.once.Do(func() {
		var copyBlocked bool
		select {
		case <-s.release:
		default:
			copyBlocked = true
		}
		s.observed <- manifestCopyObservation{
			manifestID:  s.manifestID,
			active:      s.activeAccess.Load(),
			copyBlocked: copyBlocked,
		}
	})
	select {
	case <-ctx.Done():
		return nil, false, context.Canceled
	case <-s.release:
	}
	return s.StoreOps.GetBlock(ctx, ref)
}

type startupDemandLookupObserver struct {
	waiting chan struct{}
}

func (c *startupDemandLookupObserver) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}

func (c *startupDemandLookupObserver) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/startup-demand-lookup-observer",
		controller.MustParseVersion("0.0.1"),
		"",
	)
}

func (c *startupDemandLookupObserver) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := di.GetDirective().(dex.LookupBlockFromNetwork); ok {
		select {
		case c.waiting <- struct{}{}:
		default:
		}
		return directive.R(startupDemandLookupObserverResolver{}, nil)
	}
	return nil, nil
}

func (startupDemandLookupObserverResolver) Resolve(
	ctx context.Context,
	_ directive.ResolverHandler,
) error {
	<-ctx.Done()
	return context.Canceled
}

func (c *startupDemandLookupObserver) Close() error {
	return nil
}

type startupDemandLookupObserverResolver struct{}

type testSchedulerStaticLookup struct {
	store block.StoreOps
}

// BeginReadOperation retains the fixture lookup for a bounded read.
func (l *testSchedulerStaticLookup) BeginReadOperation(context.Context) (bucket_lookup.Lookup, func(), error) {
	return l, func() {}, nil
}

func (l *testSchedulerStaticLookup) LookupBlock(
	ctx context.Context,
	ref *block.BlockRef,
	_ ...bucket_lookup.LookupBlockOption,
) ([]byte, bool, error) {
	return l.store.GetBlock(ctx, ref)
}

// LookupStoredBlock reads the block without refs because this test lookup
// keeps no ref graph.
func (l *testSchedulerStaticLookup) LookupStoredBlock(ctx context.Context, ref *block.BlockRef, opts ...bucket_lookup.LookupBlockOption) (*block.StoredBlock, error) {
	data, found, err := l.LookupBlock(ctx, ref, opts...)
	if err != nil || !found {
		return nil, err
	}
	return &block.StoredBlock{Data: data}, nil
}

func (l *testSchedulerStaticLookup) LookupBlockExistsBatch(
	ctx context.Context,
	refs []*block.BlockRef,
	_ ...bucket_lookup.LookupBlockOption,
) ([]bool, error) {
	return l.store.GetBlockExistsBatch(ctx, refs)
}

func (l *testSchedulerStaticLookup) PutBlock(
	context.Context,
	[]byte,
	*block.PutOpts,
) ([]*bucket.ObjectRef, bool, error) {
	return nil, false, bucket_lookup.ErrNotImplemented
}

type testSchedulerStaticLookupHandle struct {
	conf   *bucket.Config
	lookup bucket_lookup.Lookup
}

func (h *testSchedulerStaticLookupHandle) GetDisposed() bool {
	return false
}

func (h *testSchedulerStaticLookupHandle) GetBucketConfig() *bucket.Config {
	return h.conf
}

func (h *testSchedulerStaticLookupHandle) GetLookup(context.Context) (bucket_lookup.Lookup, error) {
	return h.lookup, nil
}

type testSchedulerStaticLookupController struct {
	bucketID string
	handle   bucket_lookup.Handle
}

func (c *testSchedulerStaticLookupController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}

func (c *testSchedulerStaticLookupController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/scheduler-static-lookup",
		controller.MustParseVersion("0.0.1"),
		"",
	)
}

func (c *testSchedulerStaticLookupController) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	d, ok := di.GetDirective().(bucket_lookup.BuildBucketLookup)
	if !ok || d.BuildBucketLookupBucketID() != c.bucketID {
		return nil, nil
	}
	return directive.R(
		directive.NewValueResolver([]bucket_lookup.BuildBucketLookupValue{c.handle}),
		nil,
	)
}

func (c *testSchedulerStaticLookupController) Close() error {
	return nil
}

type writebackLookupBlockStore struct {
	block.StoreOps
	cache       block.StoreOps
	networkGets *atomic.Uint32
}

func (s *writebackLookupBlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	// Open a read operation on the store.
	source, sourceRel, err := s.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	cache, cacheRel, err := s.cache.BeginReadOperation(ctx)
	if err != nil {
		sourceRel()
		return nil, nil, err
	}
	return &writebackLookupBlockStore{
			StoreOps:    source,
			cache:       cache,
			networkGets: s.networkGets,
		},
		func() {
			cacheRel()
			sourceRel()
		},
		nil
}

func (s *writebackLookupBlockStore) GetBlock(
	ctx context.Context,
	ref *block.BlockRef,
) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return nil, false, err
	}
	return stored.Data, true, nil
}

func (s *writebackLookupBlockStore) GetStoredBlock(
	ctx context.Context,
	ref *block.BlockRef,
) (*block.StoredBlock, error) {
	// Read the stored block from the cache.
	stored, err := s.cache.GetStoredBlock(ctx, ref)
	if err != nil || stored != nil {
		return stored, err
	}
	s.networkGets.Add(1)
	stored, err = s.StoreOps.GetStoredBlock(ctx, ref)
	if err != nil || stored == nil {
		return stored, err
	}
	_, _, err = s.cache.PutBlock(ctx, stored.Data, stored.PutOpts(ref))
	return stored, err
}

var _ block.StoreOps = (*writebackLookupBlockStore)(nil)

type countingBlockStore struct {
	store block.StoreOps
	gets  *atomic.Uint32
}

// GetRoot reads the root from the inner store.
func (s *countingBlockStore) GetRoot(ctx context.Context, name string) (*block.BlockRef, error) {
	return s.store.GetRoot(ctx, name)
}

// ListRoots lists the inner store's roots.
func (s *countingBlockStore) ListRoots(ctx context.Context, prefix string, fn func(block.NamedRoot) error) error {
	return s.store.ListRoots(ctx, prefix, fn)
}

// SetRoots moves the roots on the inner store.
func (s *countingBlockStore) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot) error {
	return s.store.SetRoots(ctx, entries, roots)
}

func (s *countingBlockStore) GetHashType() hash.HashType {
	return s.store.GetHashType()
}

func (s *countingBlockStore) GetSupportedFeatures() block.StoreFeature {
	return s.store.GetSupportedFeatures()
}

func (s *countingBlockStore) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	store, release, err := s.store.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &countingBlockStore{store: store, gets: s.gets}, release, nil
}

func (s *countingBlockStore) PutBlock(
	ctx context.Context,
	data []byte,
	opts *block.PutOpts,
) (*block.BlockRef, bool, error) {
	return s.store.PutBlock(ctx, data, opts)
}

func (s *countingBlockStore) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	return s.store.PutBlockBatch(ctx, entries)
}

func (s *countingBlockStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	s.gets.Add(1)
	return s.store.GetBlock(ctx, ref)
}

// GetStoredBlock serves the block without refs because this test store
// keeps block bytes without their refs.
func (s *countingBlockStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	return block.GetBlockWithoutRefs(ctx, s, ref)
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

func (s *countingBlockStore) Sync(ctx context.Context) (bool, error) {
	return s.store.Sync(ctx)
}

var _ block.StoreOps = (*countingBlockStore)(nil)

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
	// Mark the helper.
	t.Helper()

	// Fetch the object from the World state.
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatalf("expected object %q", objKey)
	}

	// Read the object's root ref.
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if ref == nil || ref.GetRootRef().GetEmpty() {
		t.Fatalf("expected object %q root ref", objKey)
	}

	// Corrupt the root ref hash and store it.
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

func (h *testPluginHost) ExecutePlugin(
	ctx context.Context,
	pluginID,
	instanceKey,
	executionKey,
	manifestRoot,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
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
	executionKey,
	manifestRoot,
	entrypoint string,
	pluginDist *unixfs.FSHandle,
	pluginAssets *unixfs.FSHandle,
	hostRpcMux srpc.Mux,
	rpcInit bldr_plugin_host.PluginRpcInitCb,
) error {
	// Look up the dist entrypoint file.
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

	// Look up the plugin file in the filesystem.
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

// newSchedulerTestWorld starts the manifest storage and its empty cursor.
// The caller releases the cursor before its backing testbed with the returned function.
func newSchedulerTestWorld(t *testing.T, ctx context.Context, le *logrus.Entry) (*testbed.Testbed, *bucket_lookup.Cursor, func()) {
	// Acquire storage before opening the cursor that borrows it.
	t.Helper()
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}

	// Preserve the cursor-before-testbed release order at the caller's return.
	return tb, ocs, func() {
		ocs.Release()
		tb.Release()
	}
}
