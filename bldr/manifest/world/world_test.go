package bldr_manifest_world

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/cayley/quad"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	"github.com/aperturerobotics/controllerbus/directive"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	"github.com/s4wave/spacewave/db/dex"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestNewManifestQuadLabelsConcreteManifestID(t *testing.T) {
	gq := NewManifestQuad("plugin-host", "plugin-host/ref/spacewave-web", "spacewave-web")
	want := quad.IRI("spacewave-web").String()
	if gq.GetLabel() != want {
		t.Fatalf("manifest quad label = %q, want %q", gq.GetLabel(), want)
	}
}

func TestNewManifestQuadKeepsEmptyBundleLabel(t *testing.T) {
	gq := NewManifestQuad("plugin-host", "plugin-host/bundle", "")
	if gq.GetLabel() != "" {
		t.Fatalf("manifest quad label = %q, want empty bundle/store label", gq.GetLabel())
	}
}

func TestCollectReleaseWorldManifestsForManifestID(t *testing.T) {
	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the release manifest store in the World.
	const releaseManifestKey = "spacewave/release/manifests"
	if _, err := CreateManifestStore(ctx, ws, releaseManifestKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a test manifest ref under the release manifest key.
	ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 11)
	if err := ExStoreManifestOp(
		ctx,
		ws,
		peer.ID("test"),
		"release/manifests/spacewave-web/js",
		[]string{releaseManifestKey},
		ref,
	); err != nil {
		t.Fatal(err.Error())
	}

	// Look up the manifest graph quad written by the store op.
	quads, err := ws.LookupGraphQuads(
		ctx,
		world.NewGraphQuadWithKeys(
			releaseManifestKey,
			PredManifest.String(),
			"release/manifests/spacewave-web/js",
			"",
		),
		1,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert the graph edge exists with the manifest ID label.
	if len(quads) != 1 {
		t.Fatalf("manifest graph edge count = %d", len(quads))
	}
	wantLabel := quad.IRI("spacewave-web").String()
	if quads[0].GetLabel() != wantLabel {
		t.Fatalf("manifest graph edge label = %q, want %q", quads[0].GetLabel(), wantLabel)
	}

	// Collect the manifests for the stored manifest ID.
	got, errs, err := CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		releaseManifestKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert the collected manifest matches the stored ref.
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].Manifest.GetMeta().GetManifestId() != "spacewave-web" {
		t.Fatalf("manifest id = %q", got[0].Manifest.GetMeta().GetManifestId())
	}
	if got[0].Manifest.GetMeta().GetPlatformId() != "js" {
		t.Fatalf("platform id = %q", got[0].Manifest.GetMeta().GetPlatformId())
	}
	if !got[0].ManifestRef.EqualVT(ref.GetManifestRef()) {
		t.Fatalf("manifest ref was not preserved")
	}
}

func TestCollectStartupManifestsForManifestIDsBoundsReleaseReads(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the release manifest store in the World.
	const storeKey = "release/manifests"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store one release manifest ref per manifest ID.
	manifestIDs := []string{
		"spacewave-app",
		"spacewave-cli",
		"spacewave-core",
		"spacewave-devtool",
		"spacewave-docs",
		"spacewave-forge",
		"spacewave-gateway",
		"spacewave-host",
		"spacewave-identity",
		"spacewave-notes",
		"spacewave-shell",
		"spacewave-web",
		"spacewave-world",
	}
	for i, manifestID := range manifestIDs {
		ref := createTestManifestRef(t, ctx, tb, manifestID, "js", uint64(i+1))
		if err := ExStoreManifestOp(
			ctx,
			ws,
			peer.ID("test"),
			"release/manifests/"+manifestID,
			[]string{storeKey},
			ref,
		); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Collect startup manifests for a subset of the stored manifest IDs.
	counted := &manifestSelectionCountingWorldState{WorldState: ws}
	manifests, manifestErrs, err := CollectStartupManifestsForManifestIDs(
		ctx,
		counted,
		[]string{"spacewave-core", "spacewave-web"},
		nil,
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(manifestErrs) != 0 {
		t.Fatalf("manifest errors = %v", manifestErrs)
	}
	if len(manifests) != 2 || len(manifests["spacewave-core"]) != 1 || len(manifests["spacewave-web"]) != 1 {
		t.Fatalf("selected manifests = %#v", manifests)
	}
	if got := counted.manifestReads.Load(); got != 2 {
		t.Fatalf("selected manifest reads = %d, want 2 of 13 release refs", got)
	}
	if got := counted.batchLookups.Load(); got == 0 {
		t.Fatal("selected manifest traversal did not use LookupGraphQuadsBatch")
	}
	if got := counted.cayleyTraversals.Load(); got != 0 {
		t.Fatalf("selected manifest traversal used %d complete Cayley traversals", got)
	}

	// Collect all manifests with an empty manifest ID to force the full traversal.
	all, allErrs, err := CollectStartupManifestsForManifestIDs(ctx, ws, []string{""}, nil, storeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(allErrs) != 0 || len(all) != len(manifestIDs) {
		t.Fatalf("empty-ID full traversal manifests=%d errors=%v", len(all), allErrs)
	}
}

func TestCollectStartupManifestsForManifestIDsPagesLargeStores(t *testing.T) {
	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build the mock World state from an empty cursor.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store more manifest refs than the first batch limit returns.
	const storeKey = "release/manifests"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}
	manifestIDs := make([]string, manifestEdgeLookupLimit+manifestEdgeLookupLimit/2)
	for i := range manifestIDs {
		manifestIDs[i] = fmt.Sprintf("app-%03d", i)
		ref := createTestManifestRef(t, ctx, tb, manifestIDs[i], "js", uint64(i+1))
		if err := ExStoreManifestOp(
			ctx,
			ws,
			peer.ID("test"),
			"release/manifests/"+manifestIDs[i],
			[]string{storeKey},
			ref,
		); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Collect every manifest through bounded batch lookups.
	counted := &manifestSelectionCountingWorldState{WorldState: ws}
	manifests, manifestErrs, err := CollectStartupManifestsForManifestIDs(
		ctx,
		counted,
		manifestIDs,
		nil,
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Every stored manifest is collected after the full first page.
	if len(manifestErrs) != 0 {
		t.Fatalf("manifest errors = %v", manifestErrs)
	}
	if len(manifests) != len(manifestIDs) {
		t.Fatalf("collected %d manifests, want %d", len(manifests), len(manifestIDs))
	}
	if got := counted.maxBatchLimit.Load(); got <= manifestEdgeLookupLimit {
		t.Fatalf("largest batch limit = %d, want a retry above %d", got, manifestEdgeLookupLimit)
	}
}

type manifestSelectionCountingWorldState struct {
	world.WorldState

	manifestReads    atomic.Int64
	batchLookups     atomic.Int64
	maxBatchLimit    atomic.Uint32
	cayleyTraversals atomic.Int64
}

func (w *manifestSelectionCountingWorldState) GetObject(
	ctx context.Context,
	key string,
) (world.ObjectState, bool, error) {
	obj, ok, err := w.WorldState.GetObject(ctx, key)
	if obj == nil {
		return nil, ok, err
	}
	return &manifestSelectionCountingObjectState{
		ObjectState:   obj,
		manifestReads: &w.manifestReads,
	}, ok, err
}

func (w *manifestSelectionCountingWorldState) LookupGraphQuadsBatch(
	ctx context.Context,
	filters []world.GraphQuad,
	limitPerFilter uint32,
) ([][]world.GraphQuad, error) {
	// Reject an unbounded batch as the remote World API does.
	if limitPerFilter == 0 {
		return nil, errors.New("limit_per_filter must be non-zero")
	}

	// Count the lookup and record the largest limit requested.
	w.batchLookups.Add(1)
	for {
		prev := w.maxBatchLimit.Load()
		if limitPerFilter <= prev || w.maxBatchLimit.CompareAndSwap(prev, limitPerFilter) {
			break
		}
	}
	return w.WorldState.LookupGraphQuadsBatch(ctx, filters, limitPerFilter)
}

func (w *manifestSelectionCountingWorldState) AccessCayleyGraph(
	ctx context.Context,
	write bool,
	cb func(context.Context, world.CayleyHandle) error,
) error {
	w.cayleyTraversals.Add(1)
	return w.WorldState.AccessCayleyGraph(ctx, write, cb)
}

type manifestSelectionCountingObjectState struct {
	world.ObjectState

	manifestReads *atomic.Int64
}

func (s *manifestSelectionCountingObjectState) AccessWorldState(
	ctx context.Context,
	ref *bucket.ObjectRef,
	cb func(*bucket_lookup.Cursor) error,
) error {
	s.manifestReads.Add(1)
	return s.ObjectState.AccessWorldState(ctx, ref, cb)
}

func TestCollectManifestsResetsStoreWithUnsupportedHashRef(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Seed a manifest object backed by an unsupported hash type.
	const badManifestKey = "plugin-host/manifest/bad"
	badRef := &bucket.ObjectRef{
		RootRef: block.NewBlockRef(hash.NewHash(hash.HashType(999), []byte{1, 2, 3})),
	}
	{
		createdObject, err := ws.CreateObject(ctx, badManifestKey, badRef)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	if err := world_types.SetObjectType(ctx, ws, badManifestKey, ManifestTypeID); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badManifestKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect manifests, resetting the unsupported hash ref.
	got, errs, err := CollectManifestsForManifestIDResettingUnsupportedHash(
		ctx,
		le,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert the reset removed the unsupported manifest.
	if len(errs) != 0 {
		t.Fatalf("manifest errors after reset = %v, want none", errs)
	}
	if len(got) != 0 {
		t.Fatalf("manifest count after reset = %d, want 0", len(got))
	}
	if err := CheckManifestStoreType(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}
	{
		objectState, found, err := ws.GetObject(ctx, badManifestKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		} else if found {
			t.Fatal("stale manifest object still exists after reset")
		}
	}
	candidates, err := ListManifestCandidates(ctx, ws, storeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(candidates) != 0 {
		t.Fatalf("manifest candidates after reset = %v, want none", candidates)
	}
}

func TestCollectStartupManifestsSkipsUnreadableLinkedRef(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a readable good manifest ref and link its graph edge.
	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store an unreadable bad manifest ref and link its graph edge.
	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	badRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	const badRefKey = "plugin-host/ref/missing"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect manifests with the default collector.
	defaultGot, defaultErrs, err := CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(defaultErrs) != 0 {
		t.Fatalf("default manifest errors = %v", defaultErrs)
	}
	if len(defaultGot) != 0 {
		t.Fatalf("default manifest count = %d", len(defaultGot))
	}

	// Collect manifests with the startup collector to surface the skip.
	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert the startup collector reported the unreadable ref.
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), badRefKey) {
		t.Fatalf("manifest error %q does not mention bad ref key %q", errs[0].Error(), badRefKey)
	}
	if !strings.Contains(errs[0].Error(), badRef.GetManifestRef().GetRootRef().MarshalString()) {
		t.Fatalf("manifest error %q does not mention bad root ref", errs[0].Error())
	}
	if !errors.Is(errs[0], block.ErrNotFound) {
		t.Fatalf("manifest error = %v, want block not found", errs[0])
	}

	// Assert the skip error carries the ref diagnostics.
	var skipErr *StartupManifestSkipError
	if !errors.As(errs[0], &skipErr) {
		t.Fatalf("manifest error = %T, want StartupManifestSkipError", errs[0])
	}
	if skipErr.ObjectKey != badRefKey {
		t.Fatalf("skip object key = %q, want %q", skipErr.ObjectKey, badRefKey)
	}
	if !skipErr.ObjectRef.EqualVT(badRef.GetManifestRef()) {
		t.Fatalf("skip object ref was not preserved")
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].GetRev() != 7 {
		t.Fatalf("manifest rev = %d", got[0].GetRev())
	}
	if !got[0].ManifestRef.EqualVT(goodRef.GetManifestRef()) {
		t.Fatalf("manifest ref was not preserved")
	}
}

func TestCollectStartupManifestsSkipsUnavailableBucketRef(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a readable good manifest ref and link its graph edge.
	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a bad manifest ref pointing at a missing bucket.
	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	badRef.GetManifestRef().BucketId = "missing-bucket"
	const badRefKey = "plugin-host/ref/missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifests for the manifest ID.
	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert the skip error names the missing bucket.
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !errors.Is(errs[0], bucket.ErrBucketNotFound) {
		t.Fatalf("manifest error = %v, want bucket not found", errs[0])
	}
	if !strings.Contains(errs[0].Error(), badRefKey) {
		t.Fatalf("manifest error %q does not mention bad ref key %q", errs[0].Error(), badRefKey)
	}
	if !strings.Contains(errs[0].Error(), "bucket=missing-bucket") {
		t.Fatalf("manifest error %q does not mention missing bucket", errs[0].Error())
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].GetRev() != 7 {
		t.Fatalf("manifest rev = %d", got[0].GetRev())
	}
	if !got[0].ManifestRef.EqualVT(goodRef.GetManifestRef()) {
		t.Fatalf("manifest ref was not preserved")
	}
}

func TestCollectStartupManifestsSkipsUnavailableLookupBucketBlockWithoutNetworkWait(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Add a controller that blocks network block lookups.
	ctrlRel, err := tb.Bus.AddController(ctx, startupManifestBlockingLookupController{}, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ctrlRel()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a readable good manifest ref and link its graph edge.
	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Configure a lookup-directive bucket that would block on the network.
	const lookupBucketID = "startup-lookup-bucket"
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

	// Build the lookup bucket and confirm its config loaded.
	_, _, _, err = tb.Volume.ApplyBucketConfig(ctx, bucketConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	waitCtx, waitCancel := context.WithTimeout(ctx, time.Second)
	lookupHandle, _, lookupHandleRef, err := bucket_lookup.ExBuildBucketLookup(waitCtx, tb.Bus, false, lookupBucketID, nil)
	waitCancel()
	if err != nil {
		t.Fatal(err.Error())
	}
	defer lookupHandleRef.Release()
	if lookupHandle.GetBucketConfig() == nil {
		t.Fatal("lookup bucket config was not loaded")
	}

	// Store a bad manifest ref pointing at the lookup bucket.
	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	badRef.GetManifestRef().BucketId = lookupBucketID
	badRef.GetManifestRef().RootRef.Hash.Hash[0] ^= 0xff
	const badRefKey = "plugin-host/ref/lookup-bucket-missing-block"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifests under a short context deadline.
	collectCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	got, errs, err := CollectStartupManifestsForManifestID(
		collectCtx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert the collection skipped without waiting for the network.
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !errors.Is(errs[0], block.ErrNotFound) {
		t.Fatalf("manifest error = %v, want block not found", errs[0])
	}
	if strings.Contains(errs[0].Error(), context.DeadlineExceeded.Error()) {
		t.Fatalf("manifest error waited for network lookup: %v", errs[0])
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].GetRev() != 7 {
		t.Fatalf("manifest rev = %d", got[0].GetRev())
	}
}

func TestDumpStartupManifestGraphForManifestIDIncludesRetainedRefDiagnostics(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a readable good manifest ref and link its graph edge.
	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a bad manifest ref pointing at a missing bucket.
	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	badRef.GetManifestRef().BucketId = "missing-bucket"
	const badRefKey = "plugin-host/ref/missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, badRefKey, badRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a legacy manifest ref with an empty graph label.
	legacyRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 11)
	legacyRef.GetManifestRef().BucketId = "legacy-missing-bucket"
	const legacyRefKey = "plugin-host/ref/legacy-missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, legacyRefKey, legacyRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, legacyRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Dump the startup manifest graph for the manifest ID.
	dump, err := DumpStartupManifestGraphForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert the dump contains the expected graph diagnostics.
	for _, want := range []string{
		"startup manifest graph manifest_id=spacewave-web platform_ids=js",
		"root plugin-host type=bldr/manifest-store",
		"edge plugin-host -> plugin-host/ref/good label=<spacewave-web>",
		"edge plugin-host -> plugin-host/ref/missing-bucket label=<spacewave-web>",
		"edge plugin-host -> plugin-host/ref/legacy-missing-bucket label=<empty>",
		"candidate plugin-host/ref/good type=<unknown>",
		"ref_meta=manifest_id=spacewave-web,build_type=production,platform_id=js,rev=7",
		"manifest_meta=manifest_id=spacewave-web,build_type=production,platform_id=js,rev=7",
		"candidate plugin-host/ref/missing-bucket type=<unknown>",
		"manifest_bucket=missing-bucket",
		"manifest_bucket=legacy-missing-bucket",
		"skip=bucket not found",
	} {
		if !strings.Contains(dump, want) {
			t.Fatalf("dump missing %q:\n%s", want, dump)
		}
	}
	if !strings.Contains(dump, "manifest_root="+badRef.GetManifestRef().GetRootRef().MarshalString()) {
		t.Fatalf("dump missing bad manifest root ref:\n%s", dump)
	}
	if !strings.Contains(dump, "object_root=") {
		t.Fatalf("dump missing object root refs:\n%s", dump)
	}
}

func TestDumpStartupManifestGraphDirectCandidateStaysLocalOnly(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Add a lookup observer controller that records network lookups.
	lookupObserver := &startupManifestGraphLookupObserver{called: make(chan struct{}, 1)}
	observerRel, err := tb.Bus.AddController(ctx, lookupObserver, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer observerRel()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Configure a lookup-directive bucket for the diagnostic dump.
	const lookupBucketID = "startup-graph-diagnostic-bucket"
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

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}
	directRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 13)
	directRef.GetManifestRef().BucketId = lookupBucketID
	const directKey = "plugin-host/direct/external"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), directKey, directRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, directKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Dump the startup manifest graph under a short context deadline.
	dumpCtx, cancel := context.WithTimeout(ctx, 250*time.Millisecond)
	defer cancel()
	dump, err := DumpStartupManifestGraphForManifestID(
		dumpCtx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !strings.Contains(dump, "candidate "+directKey) {
		t.Fatalf("dump missing direct candidate:\n%s", dump)
	}
	select {
	case <-lookupObserver.called:
		t.Fatalf("diagnostic graph dump invoked LookupBlockFromNetwork:\n%s", dump)
	default:
	}
}

func TestDumpStartupManifestGraphForManifestIDClassifiesProvenance(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a global release manifest candidate.
	releaseRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 10)
	const releaseKey = "release/manifests/spacewave-web/js"
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), releaseKey, []string{storeKey}, releaseRef); err != nil {
		t.Fatal(err.Error())
	}

	// Store a project build manifest candidate with a build-result marker.
	buildRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9)
	const buildKey = "project/build/spacewave-web/js"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), buildKey, buildRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := createStartupGraphBuildResultMarker(ctx, ws, buildKey); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, buildKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a space-local manifest candidate.
	spaceLocalRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 8)
	const spaceLocalKey = "spaces/test-space/plugins/generated/manifest"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), spaceLocalKey, spaceLocalRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, spaceLocalKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store an unknown-provenance manifest candidate.
	unknownRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const unknownKey = "plugin-host/ref/unknown"
	storeTestManifestRefObject(t, ctx, ws, unknownKey, unknownRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, unknownKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Dump the startup manifest graph for the manifest ID.
	dump, err := DumpStartupManifestGraphForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Assert each candidate carries its expected provenance classification.
	assertStartupGraphDumpLine(t, dump, "candidate "+releaseKey, "provenance=global-release", "derived=true", "protected=false")
	assertStartupGraphDumpLine(t, dump, "candidate "+buildKey, "provenance=project-build", "derived=true", "protected=false")
	assertStartupGraphDumpLine(t, dump, "candidate "+spaceLocalKey, "provenance=space-local-or-ephemeral", "derived=false", "protected=true")
	assertStartupGraphDumpLine(t, dump, "candidate "+unknownKey, "provenance=unknown", "derived=false", "protected=true")
}

func TestPruneStartupManifestCandidateRemovesOnlyProofGatedDerivedCandidate(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a wrong-manifest-ID candidate and link its graph edge.
	wrongIDRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 99)
	const wrongIDKey = "release/manifests/other-plugin/js"
	storeTestManifestRefObject(t, ctx, ws, wrongIDKey, wrongIDRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, wrongIDKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifest eligibility for the manifest ID.
	candidates, err := CollectStartupManifestEligibilityForManifestID(ctx, ws, "spacewave-web", []string{"js"}, storeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	candidate := findStartupCandidateByKey(t, candidates, wrongIDKey)
	if candidate.Eligibility != StartupManifestEligibilityQuarantined {
		t.Fatalf("candidate eligibility = %q, want quarantined", candidate.Eligibility)
	}

	// Prune the quarantined candidate with full proofs.
	res, err := PruneStartupManifestCandidate(
		ctx,
		ws,
		candidate,
		StartupManifestPruneProof{
			Reachability:        true,
			Quarantine:          true,
			CopiedStateRelaunch: true,
		},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !res.Pruned || !res.DeletedObject || res.DeletedEdges != 1 {
		t.Fatalf("prune result = %+v, want one edge and object deleted", res)
	}
	{
		objectState, ok, err := ws.GetObject(ctx, wrongIDKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		} else if ok {
			t.Fatal("expected proof-gated derived candidate object to be deleted")
		}
	}
	quads, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys(storeKey, PredManifest.String(), wrongIDKey, ""), 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(quads) != 0 {
		t.Fatalf("expected startup graph edge to be deleted, got %d", len(quads))
	}
}

func TestPruneStartupManifestCandidatePreservesProtectedAndUnprovenCandidates(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a protected space-local manifest candidate.
	spaceLocalRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 8)
	const spaceLocalKey = "spaces/test-space/plugins/generated/manifest"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), spaceLocalKey, spaceLocalRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, spaceLocalKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a derived release manifest candidate.
	derivedRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 9)
	const derivedKey = "release/manifests/other-plugin/js"
	storeTestManifestRefObject(t, ctx, ws, derivedKey, derivedRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, derivedKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifest eligibility for both candidates.
	candidates, err := CollectStartupManifestEligibilityForManifestID(ctx, ws, "spacewave-web", []string{"js"}, storeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	spaceLocalCandidate := findStartupCandidateByKey(t, candidates, spaceLocalKey)
	derivedCandidate := findStartupCandidateByKey(t, candidates, derivedKey)

	// Prune the protected space-local candidate with full proofs.
	res, err := PruneStartupManifestCandidate(
		ctx,
		ws,
		spaceLocalCandidate,
		StartupManifestPruneProof{
			Reachability:        true,
			Quarantine:          true,
			CopiedStateRelaunch: true,
		},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if res.Pruned || res.Reason != "source-protected:space-local-or-ephemeral" {
		t.Fatalf("space-local prune result = %+v, want protected no-op", res)
	}

	// Prune the derived candidate without the relaunch proof.
	res, err = PruneStartupManifestCandidate(
		ctx,
		ws,
		derivedCandidate,
		StartupManifestPruneProof{
			Reachability: true,
			Quarantine:   true,
		},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if res.Pruned || res.Reason != "missing-copied-state-relaunch-proof" {
		t.Fatalf("unproven derived prune result = %+v, want relaunch-proof no-op", res)
	}

	// Prune an unsafe candidate that is not quarantined.
	unsafeCandidate := &StartupManifestCandidateEligibility{
		ObjectKey:   derivedKey,
		Eligibility: StartupManifestEligibilityUnsafe,
	}
	res, err = PruneStartupManifestCandidate(
		ctx,
		ws,
		unsafeCandidate,
		StartupManifestPruneProof{
			Reachability:        true,
			Quarantine:          true,
			CopiedStateRelaunch: true,
		},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if res.Pruned || res.Reason != "not-quarantined:unsafe" {
		t.Fatalf("unsafe prune result = %+v, want unsafe no-op", res)
	}

	// Assert both candidate objects remain in the World.
	for _, key := range []string{spaceLocalKey, derivedKey} {
		{
			objectState, ok, err := ws.GetObject(ctx, key)
			world.ReleaseObjectState(objectState)
			if err != nil {
				t.Fatal(err.Error())
			} else if !ok {
				t.Fatalf("expected protected/unproven candidate %q to remain", key)
			}
		}
	}
}

func TestPruneStartupManifestCandidateRequiresExclusiveReachability(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}
	const otherStoreKey = "other-plugin-host"
	if _, err := CreateManifestStore(ctx, ws, otherStoreKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a wrong-manifest-ID candidate shared by two manifest stores.
	wrongIDRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 99)
	const wrongIDKey = "release/manifests/shared-other-plugin/js"
	storeTestManifestRefObject(t, ctx, ws, wrongIDKey, wrongIDRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, wrongIDKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(otherStoreKey, wrongIDKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect eligibility and prune the shared candidate with full proofs.
	candidates, err := CollectStartupManifestEligibilityForManifestID(ctx, ws, "spacewave-web", []string{"js"}, storeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	candidate := findStartupCandidateByKey(t, candidates, wrongIDKey)
	res, err := PruneStartupManifestCandidate(
		ctx,
		ws,
		candidate,
		StartupManifestPruneProof{
			Reachability:        true,
			Quarantine:          true,
			CopiedStateRelaunch: true,
		},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if res.Pruned || !strings.HasPrefix(res.Reason, "reachable-from-other-root:") {
		t.Fatalf("shared candidate prune result = %+v, want reachability no-op", res)
	}
	{
		objectState, ok, err := ws.GetObject(ctx, wrongIDKey)
		world.ReleaseObjectState(objectState)
		if err != nil {
			t.Fatal(err.Error())
		} else if !ok {
			t.Fatal("expected shared reachable candidate object to remain")
		}
	}
}

func TestCollectStartupManifestsForManifestIDNarrowsLabelsAndKeepsLegacyEmpty(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store an exact-label manifest ref candidate.
	exactRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const exactRefKey = "plugin-host/ref/exact"
	storeTestManifestRefObject(t, ctx, ws, exactRefKey, exactRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, exactRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a legacy empty-label manifest ref candidate.
	legacyRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 5)
	const legacyRefKey = "plugin-host/ref/legacy-empty"
	storeTestManifestRefObject(t, ctx, ws, legacyRefKey, legacyRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, legacyRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Store an unrelated manifest ref candidate with a different label.
	unrelatedRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 11)
	unrelatedRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	const unrelatedRefKey = "plugin-host/ref/unrelated"
	storeTestManifestRefObject(t, ctx, ws, unrelatedRefKey, unrelatedRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, unrelatedRefKey, "other-plugin")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifests for the manifest ID.
	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 2 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].GetRev() != 7 {
		t.Fatalf("exact manifest rev = %d", got[0].GetRev())
	}
	if got[1].GetRev() != 5 {
		t.Fatalf("legacy manifest rev = %d", got[1].GetRev())
	}
	if !got[0].ManifestRef.EqualVT(exactRef.GetManifestRef()) {
		t.Fatalf("exact manifest ref was not preserved")
	}
	if !got[1].ManifestRef.EqualVT(legacyRef.GetManifestRef()) {
		t.Fatalf("legacy manifest ref was not preserved")
	}
}

func TestCollectStartupManifestsForManifestIDCoverageMatrix(t *testing.T) {
	type setupFunc func(
		t *testing.T,
		ctx context.Context,
		tb *testbed.Testbed,
		ws world.WorldState,
		storeKey string,
	) (*manifest.ManifestRef, string)

	cases := []struct {
		name  string
		setup setupFunc
	}{
		{
			name: "direct manifest exact-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {

				// Store the direct manifest and link its exact-label graph edge.
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 31)
				const manifestKey = "plugin-host/direct/exact"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, manifestKey, "spacewave-web")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "direct manifest legacy empty-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {

				// Store the direct manifest and link its legacy empty-label edge.
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 32)
				const manifestKey = "plugin-host/direct/legacy-empty"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, manifestKey, "")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "release-world manifest exact-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 33)
				const manifestKey = "release/manifests/spacewave-web/js"
				if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{storeKey}, ref); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "release-world manifest legacy empty-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {

				// Store the release-world manifest through the store op.
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 34)
				const manifestKey = "release/manifests/spacewave-web/js/legacy-empty"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, manifestKey, "")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "bundle manifest exact-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {

				// Store the bundle manifest and link its exact-label bundle edge.
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 35)
				const manifestKey = "plugin-host/bundle/exact/manifest"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				const bundleKey = "plugin-host/bundle/exact"
				if _, _, err := CreateManifestBundle(ctx, ws, bundleKey, []string{manifestKey}, timestamp.Now()); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, bundleKey, "spacewave-web")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
		{
			name: "bundle manifest legacy empty-label edge",
			setup: func(
				t *testing.T,
				ctx context.Context,
				tb *testbed.Testbed,
				ws world.WorldState,
				storeKey string,
			) (*manifest.ManifestRef, string) {

				// Store the bundle manifest and link its legacy empty-label edges.
				ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 36)
				const manifestKey = "plugin-host/bundle/legacy-empty/manifest"
				if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
					t.Fatal(err.Error())
				}
				const bundleKey = "plugin-host/bundle/legacy-empty"
				if _, _, err := CreateManifestBundle(ctx, ws, bundleKey, []string{manifestKey}, timestamp.Now()); err != nil {
					t.Fatal(err.Error())
				}

				// Rewire the bundle edges to the legacy empty label.
				if err := ws.DeleteGraphQuad(ctx, NewManifestQuad(bundleKey, manifestKey, "spacewave-web")); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(bundleKey, manifestKey, "")); err != nil {
					t.Fatal(err.Error())
				}
				if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, bundleKey, "")); err != nil {
					t.Fatal(err.Error())
				}
				return ref, manifestKey
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			// Set up the test context and logger.
			ctx := context.Background()
			le := logrus.NewEntry(logrus.New())

			// Start a testbed holding the mock World.
			tb, err := testbed.NewTestbed(ctx, le)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer tb.Release()

			// Build an empty cursor for the mock World state.
			ocs, err := tb.BuildEmptyCursor(ctx)
			if err != nil {
				t.Fatal(err.Error())
			}
			defer ocs.Release()

			// Build the mock World state from the empty cursor.
			ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
			if err != nil {
				t.Fatal(err.Error())
			}

			// Create the plugin-host manifest store in the World.
			const storeKey = "plugin-host"
			if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
				t.Fatal(err.Error())
			}

			// Run the case setup and collect startup manifests for the manifest ID.
			wantRef, wantManifestKey := tc.setup(t, ctx, tb, ws, storeKey)
			got, errs, err := CollectStartupManifestsForManifestID(
				ctx,
				ws,
				"spacewave-web",
				[]string{"js"},
				storeKey,
			)
			if err != nil {
				t.Fatal(err.Error())
			}
			if len(errs) != 0 {
				t.Fatalf("manifest errors = %v", errs)
			}
			if len(got) != 1 {
				t.Fatalf("manifest count = %d", len(got))
			}
			if got[0].ManifestKey != wantManifestKey {
				t.Fatalf("manifest key = %q, want %q", got[0].ManifestKey, wantManifestKey)
			}
			if !got[0].ManifestRef.EqualVT(wantRef.GetManifestRef()) {
				t.Fatalf("manifest ref was not preserved")
			}
		})
	}
}

func TestCollectStartupManifestsForManifestIDSkipsRefMetadataMismatchesBeforeOpen(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a readable good manifest ref and link its graph edge.
	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a wrong-manifest-ID ref candidate and link its graph edge.
	wrongIDRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 11)
	wrongIDRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	const wrongIDRefKey = "plugin-host/ref/wrong-id"
	storeTestManifestRefObject(t, ctx, ws, wrongIDRefKey, wrongIDRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, wrongIDRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a wrong-platform ref candidate and link its graph edge.
	wrongPlatformRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "desktop/linux/amd64", 13)
	wrongPlatformRef.ManifestRef.RootRef.Hash.Hash[0] ^= 0xff
	const wrongPlatformRefKey = "plugin-host/ref/wrong-platform"
	storeTestManifestRefObject(t, ctx, ws, wrongPlatformRefKey, wrongPlatformRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, wrongPlatformRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifests for the manifest ID.
	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if !got[0].ManifestRef.EqualVT(goodRef.GetManifestRef()) {
		t.Fatalf("selected manifest ref = %v, want good ref", got[0].ManifestRef)
	}
}

func TestCollectStartupManifestEligibilityClassifiesRetainedCandidates(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}
	const nestedStoreKey = "plugin-host/retained-store"
	if _, err := CreateManifestStore(ctx, ws, nestedStoreKey); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, nestedStoreKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Store an exact-label manifest ref candidate.
	exactRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const exactRefKey = "plugin-host/ref/exact"
	storeTestManifestRefObject(t, ctx, ws, exactRefKey, exactRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, exactRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a legacy empty-label manifest ref candidate.
	legacyRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 6)
	const legacyRefKey = "plugin-host/ref/legacy"
	storeTestManifestRefObject(t, ctx, ws, legacyRefKey, legacyRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, legacyRefKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a wrong-manifest-ID manifest ref candidate.
	wrongIDRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 5)
	const wrongIDRefKey = "plugin-host/ref/wrong-id"
	storeTestManifestRefObject(t, ctx, ws, wrongIDRefKey, wrongIDRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, wrongIDRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a wrong-platform manifest ref candidate.
	wrongPlatformRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "desktop/linux/amd64", 4)
	const wrongPlatformRefKey = "plugin-host/ref/wrong-platform"
	storeTestManifestRefObject(t, ctx, ws, wrongPlatformRefKey, wrongPlatformRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, wrongPlatformRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Store a missing-bucket manifest ref candidate.
	missingBucketRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 3)
	missingBucketRef.ManifestRef.BucketId = "missing-retained-bucket"
	const missingBucketRefKey = "plugin-host/ref/missing-bucket"
	storeTestManifestRefObject(t, ctx, ws, missingBucketRefKey, missingBucketRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, missingBucketRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifest eligibility for the manifest ID.
	got, err := CollectStartupManifestEligibilityForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Index the collected candidates by object key.
	byKey := make(map[string]*StartupManifestCandidateEligibility, len(got))
	for _, candidate := range got {
		byKey[candidate.ObjectKey] = candidate
	}
	assertStartupEligibility(t, byKey, exactRefKey, StartupManifestEligibilityEligible, "exact-label")
	assertStartupEligibility(t, byKey, legacyRefKey, StartupManifestEligibilityCompatibleLegacy, "legacy-empty-label")
	assertStartupEligibility(t, byKey, wrongIDRefKey, StartupManifestEligibilityQuarantined, "manifest-id-mismatch:other-plugin")
	assertStartupEligibility(t, byKey, wrongPlatformRefKey, StartupManifestEligibilityIgnored, "platform-filtered:desktop/linux/amd64")
	assertStartupEligibility(t, byKey, nestedStoreKey, StartupManifestEligibilityIgnored, "intermediate:bldr/manifest-store")
	assertStartupEligibility(t, byKey, missingBucketRefKey, StartupManifestEligibilityUnsafe, "manifest-ref-unreadable:")

	// Summarize the eligibility classification results.
	summary := SummarizeStartupManifestEligibility(got, 3)
	if !strings.Contains(summary, "eligible exact-label") {
		t.Fatalf("summary missing eligible item: %q", summary)
	}
	if !strings.Contains(summary, "+3 more") {
		t.Fatalf("summary missing truncation: %q", summary)
	}
}

func TestCollectStartupManifestsForManifestIDRejectsDecodedMetadataMismatch(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a ref whose decoded metadata mismatches its manifest meta.
	decodedOtherRef := createTestManifestRef(t, ctx, tb, "other-plugin", "js", 11)
	refHint := manifest.NewManifestRef(
		&manifest.ManifestMeta{
			ManifestId: "spacewave-web",
			BuildType:  "production",
			PlatformId: "js",
			Rev:        11,
		},
		decodedOtherRef.GetManifestRef(),
	)
	const refHintKey = "plugin-host/ref/decoded-mismatch"
	storeTestManifestRefObject(t, ctx, ws, refHintKey, refHint)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, refHintKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifests for the manifest ID.
	got, errs, err := CollectStartupManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(got) != 0 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "manifest ref meta does not match manifest meta") {
		t.Fatalf("manifest error = %v, want decoded metadata mismatch", errs[0])
	}
}

func TestCollectStartupManifestsRejectsInvalidDecodedMetadata(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a manifest with invalid decoded metadata.
	invalidRef := createTestManifestRef(t, ctx, tb, "Spacewave-Web", "js", 11)
	const invalidManifestKey = "plugin-host/manifest/invalid"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), invalidManifestKey, invalidRef.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, invalidManifestKey, "")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect startup manifests across the store.
	got, errs, err := CollectStartupManifests(ctx, ws, []string{"js"}, storeKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(got) != 0 {
		t.Fatalf("manifest set count = %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), "manifest_id") {
		t.Fatalf("manifest error = %v, want invalid manifest metadata", errs[0])
	}
}

func TestStartupManifestSkipErrorIncludesBucketDiagnostics(t *testing.T) {
	err := newStartupManifestSkipError(
		"plugin-host/ref/missing-bucket",
		&bucket.ObjectRef{BucketId: "missing-bucket"},
		bucket.ErrBucketNotFound,
	)
	if !errors.Is(err, bucket.ErrBucketNotFound) {
		t.Fatalf("skip error = %v, want bucket not found", err)
	}
	if !strings.Contains(err.Error(), "bucket=missing-bucket") {
		t.Fatalf("skip error %q does not mention missing bucket", err.Error())
	}
}

func TestStartupContextErrorClassifiesFatalContextErrors(t *testing.T) {
	if got := startupContextError(context.Canceled); !errors.Is(got, context.Canceled) {
		t.Fatalf("canceled error = %v, want context canceled", got)
	}
	if got := startupContextError(context.DeadlineExceeded); !errors.Is(got, context.DeadlineExceeded) {
		t.Fatalf("deadline error = %v, want deadline exceeded", got)
	}
	if got := startupContextError(bucket.ErrBucketNotFound); got != nil {
		t.Fatalf("availability error = %v, want nil fatal context error", got)
	}
}

func TestCollectManifestsReportsUnreadableManifestObject(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Store a bad manifest ref with a corrupted root hash.
	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9).GetManifestRef().CloneVT()
	badRef.RootRef.Hash.Hash[0] ^= 0xff
	const badManifestKey = "plugin-host/manifest/bad"
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), badManifestKey, badRef); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badManifestKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	// Collect manifests for the manifest ID.
	got, errs, err := CollectManifestsForManifestID(
		ctx,
		ws,
		"spacewave-web",
		[]string{"js"},
		storeKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(got) != 0 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), badManifestKey) {
		t.Fatalf("manifest error %q does not mention bad manifest key %q", errs[0].Error(), badManifestKey)
	}
}

func TestCollectDirectManifestForManifestID(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store a direct manifest and link its self graph edge.
	const manifestKey = "glados-core"
	ref := createTestManifestRef(t, ctx, tb, manifestKey, "js", 7)
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(manifestKey, manifestKey, manifestKey)); err != nil {
		t.Fatal(err.Error())
	}

	// Collect manifests for the manifest ID.
	got, errs, err := CollectManifestsForManifestID(
		ctx,
		ws,
		manifestKey,
		[]string{"js"},
		manifestKey,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
	if got[0].Manifest.GetMeta().GetManifestId() != manifestKey {
		t.Fatalf("manifest id = %q", got[0].Manifest.GetMeta().GetManifestId())
	}
	if !got[0].ManifestRef.EqualVT(ref.GetManifestRef()) {
		t.Fatalf("manifest ref was not preserved")
	}
}

func TestManifestObjectRefsSameExecutableMatchesInlineAndReferencedTransformConf(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build the inline transform conf and a matching manifest ref.
	transformConf := newTestManifestTransformConf(t)
	inlineManifest := createTestManifestRefWithTransformConf(t, ctx, tb, "spacewave-web", "js", 7, transformConf)
	inlineRef := inlineManifest.GetManifestRef().CloneVT()
	inlineRef.BucketId = tb.BucketId
	inlineRef.TransformConfRef = nil
	if inlineRef.GetRootRef().GetEmpty() {
		t.Fatal("test setup: manifest root ref is empty")
	}
	if inlineRef.GetTransformConf().GetEmpty() {
		t.Fatal("test setup: inline transform conf is empty")
	}
	if !inlineRef.GetTransformConfRef().GetEmpty() {
		t.Fatalf("test setup: inline transform conf ref = %s, want empty", inlineRef.GetTransformConfRef().MarshalString())
	}

	// Build a referenced transform conf encoding of the same manifest.
	referencedRef := inlineRef.CloneVT()
	referencedRef.BucketId = "dist/spacewave"
	referencedRef.TransformConfRef = writeTestTransformConfRef(t, ctx, tb, "dist/spacewave", inlineRef.GetTransformConf())
	referencedRef.TransformConf = nil
	if referencedRef.GetTransformConfRef().GetEmpty() {
		t.Fatal("test setup: referenced transform conf ref is empty")
	}
	if referencedRef.TransformConf != nil {
		t.Fatal("test setup: referenced transform conf should not be inline")
	}
	if !inlineRef.GetRootRef().EqualsRef(referencedRef.GetRootRef()) {
		t.Fatal("test setup: referenced manifest root differs from inline root")
	}

	// Assert both encodings count as the same executable.
	if !ManifestObjectRefsSameExecutable(inlineRef, referencedRef) {
		t.Fatal("same manifest root with local inline transform conf and external referenced transform conf was not treated as the same executable")
	}
}

func TestSetManifestBucketRelocationDoesNotBumpLinkedRev(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	var worldBucketID string
	if err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		worldBucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Create the manifest key and the shared transform conf.
	const manifestKey = "plugin-host/ref/spacewave-web/js"
	transformConf := newTestManifestTransformConf(t)

	// Seed the local manifest using the exact local inline executable ref that a
	// download-before-swap warm start should preserve when a fetched external ref
	// has the same executable identity.
	localRef := createTestManifestRefWithTransformConf(t, ctx, tb, "spacewave-web", "js", 7, transformConf)
	localManifestRef := localRef.GetManifestRef()
	localManifestRef.BucketId = worldBucketID
	localManifestRef.TransformConfRef = nil
	if localManifestRef.GetTransformConf().GetEmpty() {
		t.Fatal("test setup: local manifest transform conf is empty")
	}
	if !localManifestRef.GetTransformConfRef().GetEmpty() {
		t.Fatalf("test setup: local manifest transform conf ref = %s, want empty inline encoding", localManifestRef.GetTransformConfRef().MarshalString())
	}
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{storeKey}, localRef); err != nil {
		t.Fatal(err.Error())
	}
	seededRev := objectRev(t, ctx, ws, storeKey)

	// A fetched dist manifest may encode the same transform configuration by
	// reference. That exact prod divergence is not an executable manifest
	// change and must not replace the already-local inline ref.
	fetchedRef := localRef.CloneVT()
	fetchedManifestRef := fetchedRef.GetManifestRef()
	fetchedManifestRef.BucketId = "dist/spacewave"
	fetchedManifestRef.TransformConfRef = writeTestTransformConfRef(t, ctx, tb, "dist/spacewave", localManifestRef.GetTransformConf())
	fetchedManifestRef.TransformConf = nil

	// Assert the fetched ref keeps the same executable identity.
	if fetchedManifestRef.GetTransformConfRef().GetEmpty() {
		t.Fatal("test setup: fetched manifest transform conf ref is empty")
	}
	if fetchedManifestRef.TransformConf != nil {
		t.Fatal("test setup: fetched manifest transform conf should not be inline")
	}
	if !localManifestRef.GetRootRef().EqualsRef(fetchedManifestRef.GetRootRef()) {
		t.Fatal("test setup: fetched manifest root differs from local root")
	}
	if !ManifestObjectRefsSameExecutable(localManifestRef, fetchedManifestRef) {
		t.Fatal("test setup: fetched manifest should have the same executable identity as the local manifest")
	}
	if localManifestRef.EqualVT(fetchedManifestRef) {
		t.Fatal("test setup: fetched manifest ref should differ by bucket and transform encoding")
	}

	// Apply the fetched ref and assert the linked rev is unchanged.
	if _, changed, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, fetchedManifestRef); err != nil {
		t.Fatal(err.Error())
	} else if changed {
		t.Fatal("identity-equal external manifest ref was reported as changed")
	}

	// Assert the stored ref kept the local inline encoding.
	storedRef := objectRootRef(t, ctx, ws, manifestKey)
	if !storedRef.GetRootRef().EqualsRef(localManifestRef.GetRootRef()) {
		t.Fatal("stored manifest root ref changed after identity-equal external SetManifest")
	}
	if got := storedRef.GetBucketId(); got != worldBucketID {
		t.Fatalf("stored manifest ref bucket after identity-equal external SetManifest = %q, want local world bucket %q", got, worldBucketID)
	}
	if !storedRef.GetTransformConfRef().GetEmpty() {
		t.Fatalf("stored manifest transform conf ref after identity-equal external SetManifest = %s, want empty inline encoding", storedRef.GetTransformConfRef().MarshalString())
	}
	if !storedRef.GetTransformConf().EqualVT(localManifestRef.GetTransformConf()) {
		t.Fatal("stored manifest transform conf did not preserve local inline encoding after identity-equal external SetManifest")
	}

	// Store the fetched external ref and assert the linked rev is unchanged.
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{storeKey}, fetchedRef); err != nil {
		t.Fatal(err.Error())
	}
	if got := objectRev(t, ctx, ws, storeKey); got != seededRev {
		t.Fatalf("linked store rev after equivalent transform encoding relocation = %d, want unchanged %d", got, seededRev)
	}
	storedRef = objectRootRef(t, ctx, ws, manifestKey)
	if !storedRef.GetRootRef().EqualsRef(localManifestRef.GetRootRef()) {
		t.Fatal("stored manifest root ref changed after identity-equal external store op")
	}
	if got := storedRef.GetBucketId(); got != worldBucketID {
		t.Fatalf("stored manifest ref bucket after identity-equal external store op = %q, want local world bucket %q", got, worldBucketID)
	}
	if !storedRef.GetTransformConfRef().GetEmpty() {
		t.Fatalf("stored manifest transform conf ref after identity-equal external store op = %s, want empty inline encoding", storedRef.GetTransformConfRef().MarshalString())
	}
	if !storedRef.GetTransformConf().EqualVT(localManifestRef.GetTransformConf()) {
		t.Fatal("stored manifest transform conf did not preserve local inline encoding after identity-equal external store op")
	}

	// A genuinely different executable (different manifest content, so a
	// different root ref) must swap to the new local ref and bump the linked
	// store rev.
	newExecutableRef := createTestManifestRefWithTransformConf(t, ctx, tb, "spacewave-web", "js", 8, transformConf)
	newExecutableManifestRef := newExecutableRef.GetManifestRef()
	newExecutableManifestRef.BucketId = worldBucketID
	newExecutableManifestRef.TransformConfRef = nil
	if ManifestObjectRefsSameExecutable(localManifestRef, newExecutableManifestRef) {
		t.Fatal("test setup: expected a distinct executable root ref")
	}
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{storeKey}, newExecutableRef); err != nil {
		t.Fatal(err.Error())
	}

	// Store the new executable and assert the linked rev bumps.
	if got := objectRev(t, ctx, ws, storeKey); got <= seededRev {
		t.Fatalf("linked store rev after executable change = %d, want > %d", got, seededRev)
	}
	storedRef = objectRootRef(t, ctx, ws, manifestKey)
	if !storedRef.GetRootRef().EqualsRef(newExecutableManifestRef.GetRootRef()) {
		t.Fatal("stored manifest root ref did not update to the different executable")
	}
	if got := storedRef.GetBucketId(); got != worldBucketID {
		t.Fatalf("stored manifest ref bucket after executable change = %q, want local world bucket %q", got, worldBucketID)
	}
	if !storedRef.GetTransformConfRef().GetEmpty() {
		t.Fatalf("stored manifest transform conf ref after executable change = %s, want empty inline encoding", storedRef.GetTransformConfRef().MarshalString())
	}
	if !storedRef.GetTransformConf().EqualVT(newExecutableManifestRef.GetTransformConf()) {
		t.Fatal("stored manifest transform conf did not update to the new local inline encoding")
	}
}

func TestSetManifestLocalCopyNotifiesLinkedStore(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start a testbed holding the mock World.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Build an empty cursor for the mock World state.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ocs.Release()

	// Build the mock World state from the empty cursor.
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	var worldBucketID string
	if err := ws.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
		worldBucketID = cursor.GetOpArgs().GetBucketId()
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Create the plugin-host manifest store in the World.
	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	// Create the manifest key and the shared transform conf.
	const manifestKey = "plugin-host/ref/spacewave-web/js"
	transformConf := newTestManifestTransformConf(t)
	baseRef := createTestManifestRefWithTransformConf(t, ctx, tb, "spacewave-web", "js", 7, transformConf)

	// Build the external referenced-transform encoding of the manifest.
	externalRef := baseRef.CloneVT()
	externalManifestRef := externalRef.GetManifestRef()
	externalManifestRef.BucketId = "dist/spacewave"
	externalManifestRef.TransformConfRef = writeTestTransformConfRef(t, ctx, tb, "dist/spacewave", externalManifestRef.GetTransformConf())
	externalManifestRef.TransformConf = nil
	if externalManifestRef.GetTransformConfRef().GetEmpty() {
		t.Fatal("test setup: external manifest transform conf ref is empty")
	}
	if externalManifestRef.TransformConf != nil {
		t.Fatal("test setup: external manifest transform conf should not be inline")
	}

	// Build the local inline-transform encoding of the manifest.
	localRef := baseRef.CloneVT()
	localManifestRef := localRef.GetManifestRef()
	localManifestRef.BucketId = worldBucketID
	localManifestRef.TransformConfRef = nil
	if localManifestRef.GetTransformConf().GetEmpty() {
		t.Fatal("test setup: local manifest transform conf is empty")
	}
	if !localManifestRef.GetRootRef().EqualsRef(externalManifestRef.GetRootRef()) {
		t.Fatal("test setup: local manifest root differs from external root")
	}
	if !ManifestObjectRefsSameExecutable(externalManifestRef, localManifestRef) {
		t.Fatal("test setup: local manifest should have the same executable identity as the external manifest")
	}
	if externalManifestRef.EqualVT(localManifestRef) {
		t.Fatal("test setup: local manifest ref should differ by bucket and transform encoding")
	}

	// Seed the external manifest and check its stored encoding.
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{storeKey}, externalRef); err != nil {
		t.Fatal(err.Error())
	}
	seededRev := objectRev(t, ctx, ws, storeKey)
	storedRef := objectRootRef(t, ctx, ws, manifestKey)
	if got := storedRef.GetBucketId(); got != "dist/spacewave" {
		t.Fatalf("test setup: seeded manifest ref bucket = %q, want dist/spacewave", got)
	}
	if storedRef.GetTransformConfRef().GetEmpty() {
		t.Fatal("test setup: seeded manifest transform conf ref is empty")
	}

	// Store the local copy and assert the linked store rev bumps.
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{storeKey}, localRef); err != nil {
		t.Fatal(err.Error())
	}
	if got := objectRev(t, ctx, ws, storeKey); got != seededRev+1 {
		t.Fatalf("linked store rev after local copy = %d, want %d", got, seededRev+1)
	}
	storedRef = objectRootRef(t, ctx, ws, manifestKey)
	if !storedRef.GetRootRef().EqualsRef(localManifestRef.GetRootRef()) {
		t.Fatal("stored manifest root ref changed during identity-equal local upgrade")
	}
	if got := storedRef.GetBucketId(); got != worldBucketID {
		t.Fatalf("stored manifest ref bucket after identity-equal local upgrade = %q, want local world bucket %q", got, worldBucketID)
	}
	if !storedRef.GetTransformConfRef().GetEmpty() {
		t.Fatalf("stored manifest transform conf ref after identity-equal local upgrade = %s, want empty inline encoding", storedRef.GetTransformConfRef().MarshalString())
	}
	if !storedRef.GetTransformConf().EqualVT(localManifestRef.GetTransformConf()) {
		t.Fatal("stored manifest transform conf did not update to the local inline encoding")
	}
	if !ManifestObjectRefsSameExecutable(externalManifestRef, storedRef) {
		t.Fatal("stored local manifest no longer has the external manifest executable identity")
	}
}

func createTestManifestRef(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	manifestID string,
	platformID string,
	rev uint64,
) *manifest.ManifestRef {

	// Build the manifest meta for the test ref.
	t.Helper()

	// Build a cursor in the testbed bucket and write the manifest block.
	meta := &manifest.ManifestMeta{
		ManifestId: manifestID,
		BuildType:  "production",
		PlatformId: platformID,
		Rev:        rev,
	}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	// Write the manifest block and return the manifest ref.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(manifest.NewManifest(meta, "entrypoint"), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(rootRef)
	return manifest.NewManifestRef(meta, oc.GetRef())
}

func newTestManifestTransformConf(t *testing.T) *block_transform.Config {
	t.Helper()

	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	return transformConf
}

func createTestManifestRefWithTransformConf(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	manifestID string,
	platformID string,
	rev uint64,
	transformConf *block_transform.Config,
) *manifest.ManifestRef {

	// Build the manifest meta for the test ref.
	t.Helper()

	// Build a transformed cursor and write the manifest block.
	meta := &manifest.ManifestMeta{
		ManifestId: manifestID,
		BuildType:  "production",
		PlatformId: platformID,
		Rev:        rev,
	}
	oc, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		tb.Logger,
		tb.StepFactorySet,
		tb.BucketId,
		tb.Volume.GetID(),
		transformConf,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer oc.Release()

	// Write the manifest block and return the manifest ref.
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(manifest.NewManifest(meta, "entrypoint"), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(rootRef)
	return manifest.NewManifestRef(meta, oc.GetRef())
}

func writeTestTransformConfRef(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	bucketID string,
	transformConf *block_transform.Config,
) *block.BlockRef {

	// Apply the target bucket config to the testbed volume.
	t.Helper()

	// Build a cursor in the target bucket and write the transform conf.
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

	// Marshal the transform conf and store it as a block.
	transformConfData, err := bucket_lookup.MarshalTransformConf(transformConf)
	if err != nil {
		t.Fatal(err.Error())
	}
	transformConfRef, _, err := oc.PutBlock(ctx, transformConfData, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	return transformConfRef
}

func storeTestManifestRefObject(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	ref *manifest.ManifestRef,
) {
	t.Helper()

	if _, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(ref.CloneVT(), true)
		return nil
	}); err != nil {
		t.Fatal(err.Error())
	}
}

func objectRev(t *testing.T, ctx context.Context, ws world.WorldState, objKey string) uint64 {

	// Mark the helper as a test helper.
	t.Helper()
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatalf("object %q not found", objKey)
	}
	_, rev, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	return rev
}

func objectRootRef(t *testing.T, ctx context.Context, ws world.WorldState, objKey string) *bucket.ObjectRef {

	// Mark the helper as a test helper.
	t.Helper()
	obj, ok, err := ws.GetObject(ctx, objKey)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !ok {
		t.Fatalf("object %q not found", objKey)
	}
	ref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	return ref
}

func createStartupGraphBuildResultMarker(ctx context.Context, ws world.WorldState, manifestKey string) error {

	// Write the build-result marker manifest as a World object.
	ref, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(manifest.NewManifest(manifest.NewManifestMeta("build-result-marker", manifest.BuildType_DEV, "js", 1), "entrypoint"), true)
		return nil
	})
	if err != nil {
		return err
	}
	objKey := manifestKey + "/build-result"
	{
		createdObject, err := ws.CreateObject(ctx, objKey, ref)
		world.ReleaseObjectState(createdObject)
		if err != nil {
			return err
		}
	}
	return world_types.SetObjectType(ctx, ws, objKey, "bldr/manifest-build-result")
}

func assertStartupGraphDumpLine(t *testing.T, dump string, prefix string, parts ...string) {
	t.Helper()
	for line := range strings.SplitSeq(dump, "\n") {
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		for _, part := range parts {
			if !strings.Contains(line, part) {
				t.Fatalf("dump line %q missing %q", line, part)
			}
		}
		return
	}
	t.Fatalf("dump missing line prefix %q:\n%s", prefix, dump)
}

func findStartupCandidateByKey(
	t *testing.T,
	candidates []*StartupManifestCandidateEligibility,
	key string,
) *StartupManifestCandidateEligibility {
	t.Helper()
	for _, candidate := range candidates {
		if candidate != nil && candidate.ObjectKey == key {
			return candidate
		}
	}
	t.Fatalf("missing startup manifest candidate %q", key)
	return nil
}

func assertStartupEligibility(
	t *testing.T,
	byKey map[string]*StartupManifestCandidateEligibility,
	key string,
	want StartupManifestEligibility,
	reasonPrefix string,
) {

	// Mark the helper as a test helper.
	t.Helper()

	// Look up the candidate by key and assert its classification.
	candidate := byKey[key]
	if candidate == nil {
		t.Fatalf("missing startup manifest candidate %q", key)
	}
	if candidate.Eligibility != want {
		t.Fatalf("candidate %q eligibility = %q, want %q", key, candidate.Eligibility, want)
	}
	if !strings.HasPrefix(candidate.Reason, reasonPrefix) {
		t.Fatalf("candidate %q reason = %q, want prefix %q", key, candidate.Reason, reasonPrefix)
	}
}

type startupManifestBlockingLookupController struct{}

func (startupManifestBlockingLookupController) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}

func (startupManifestBlockingLookupController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/startup-manifest-blocking-lookup",
		controller.MustParseVersion("0.0.1"),
		"",
	)
}

func (startupManifestBlockingLookupController) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := di.GetDirective().(dex.LookupBlockFromNetwork); !ok {
		return nil, nil
	}
	return directive.R(startupManifestBlockingLookupResolver{}, nil)
}

func (startupManifestBlockingLookupController) Close() error {
	return nil
}

type startupManifestBlockingLookupResolver struct{}

func (startupManifestBlockingLookupResolver) Resolve(
	ctx context.Context,
	_ directive.ResolverHandler,
) error {
	<-ctx.Done()
	return context.Canceled
}

type startupManifestGraphLookupObserver struct {
	called chan struct{}
}

func (c *startupManifestGraphLookupObserver) Execute(ctx context.Context) error {
	<-ctx.Done()
	return context.Canceled
}

func (c *startupManifestGraphLookupObserver) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		"test/startup-manifest-graph-lookup-observer",
		controller.MustParseVersion("0.0.1"),
		"",
	)
}

func (c *startupManifestGraphLookupObserver) HandleDirective(
	_ context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	if _, ok := di.GetDirective().(dex.LookupBlockFromNetwork); !ok {
		return nil, nil
	}
	select {
	case c.called <- struct{}{}:
	default:
	}
	return directive.R(startupManifestBlockingLookupResolver{}, nil)
}

func (c *startupManifestGraphLookupObserver) Close() error {
	return nil
}
