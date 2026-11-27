package bldr_manifest_world

import (
	"context"
	"testing"

	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// TestExStoreManifestOpSkipsStoredManifest checks that storing an already
// stored and linked manifest applies no World operation, and that a missing
// link still applies one.
func TestExStoreManifestOpSkipsStoredManifest(t *testing.T) {
	// Start a testbed.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
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
	mockWS, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create two manifest stores and count operations applied to the World.
	const hostKey, otherKey = "plugin-host", "other-host"
	for _, key := range []string{hostKey, otherKey} {
		if _, err := CreateManifestStore(ctx, mockWS, key); err != nil {
			t.Fatal(err.Error())
		}
	}
	ws := &opCountingWorldState{WorldState: mockWS}

	// Store the manifest once, then store it again unchanged.
	ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 3)
	const manifestKey = "plugin-host/spacewave-web"
	for range 2 {
		if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey}, ref); err != nil {
			t.Fatal(err.Error())
		}
	}
	if ws.applied != 1 {
		t.Fatalf("applied %d operations storing one manifest twice, want 1", ws.applied)
	}

	// A new link applies an operation for the stored manifest.
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey, otherKey}, ref); err != nil {
		t.Fatal(err.Error())
	}
	if ws.applied != 2 {
		t.Fatalf("applied %d operations after adding a link, want 2", ws.applied)
	}
}

// TestExStoreManifestOpStoresReleaseAuthorization checks that an authorized
// ManifestRef is stored once, collected with its authorization, and stored
// again when only the authorization changes.
func TestExStoreManifestOpStoresReleaseAuthorization(t *testing.T) {
	// Start a testbed.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
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
	mockWS, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the manifest store and count operations applied to the World.
	const hostKey, manifestKey = "release", "release/spacewave-web"
	if _, err := CreateManifestStore(ctx, mockWS, hostKey); err != nil {
		t.Fatal(err.Error())
	}
	ws := &opCountingWorldState{WorldState: mockWS}

	// Sign the manifest reference with a fresh release key on each call.
	ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 3)
	signRef := func() {
		key, _, err := crypto.GenerateEd25519Key(nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := ref.SignReleaseAuthorization(key); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Store the authorized manifest twice.
	signRef()
	for range 2 {
		if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey}, ref); err != nil {
			t.Fatal(err.Error())
		}
	}
	if ws.applied != 1 {
		t.Fatalf("applied %d operations storing one authorized manifest twice, want 1", ws.applied)
	}

	// Collect the manifest with the stored authorization.
	collected, errs, err := CollectManifests(ctx, ws, nil, hostKey)
	if err != nil || len(errs) != 0 {
		t.Fatalf("collect manifests: %v %v", err, errs)
	}
	got := collected["spacewave-web"]
	if len(got) != 1 || !got[0].ManifestRef.EqualVT(ref.GetManifestRef()) {
		t.Fatalf("collected %v, want the stored manifest", got)
	}
	if !got[0].ReleaseAuthorization.EqualVT(ref.GetReleaseAuthorization()) {
		t.Fatal("collected manifest lost its release authorization")
	}

	// A new authorization of the same manifest applies an operation.
	signRef()
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey}, ref); err != nil {
		t.Fatal(err.Error())
	}
	if ws.applied != 2 {
		t.Fatalf("applied %d operations after a new authorization, want 2", ws.applied)
	}
}

// newReleaseTestWorld builds a World with a manifest store at hostKey.
func newReleaseTestWorld(t *testing.T, hostKey string) (context.Context, *testbed.Testbed, world.WorldState) {
	// Start a testbed.
	t.Helper()
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	// Build the mock World state from an empty cursor.
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(ocs.Release)
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the manifest store.
	if _, err := CreateManifestStore(ctx, ws, hostKey); err != nil {
		t.Fatal(err.Error())
	}
	return ctx, tb, ws
}

// TestExStoreManifestOpKeepsAuthorizationOfSameManifest checks that storing an
// unauthorized ref of a manifest onto its authorized ManifestRef object, as a
// download does when it copies the manifest into the World bucket, keeps the
// authorization and the ManifestRef object type.
func TestExStoreManifestOpKeepsAuthorizationOfSameManifest(t *testing.T) {
	// Build a World with a manifest store.
	const hostKey, manifestKey = "release", "release/spacewave-web"
	ctx, tb, ws := newReleaseTestWorld(t, hostKey)

	// Store the authorized manifest at an external bucket.
	ref := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 3)
	key, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := ref.SignReleaseAuthorization(key); err != nil {
		t.Fatal(err.Error())
	}
	external := ref.CloneVT()
	external.ManifestRef.BucketId = "external-bucket"
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey}, external); err != nil {
		t.Fatal(err.Error())
	}

	// Store the same manifest again without an authorization, in the World bucket.
	local := ref.CloneVT()
	local.ReleaseAuthorization = nil
	local.ManifestRef.BucketId = ""
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey}, local); err != nil {
		t.Fatal(err.Error())
	}

	// The object stays an authorized ManifestRef at the World bucket.
	if err := world_types.CheckObjectType(ctx, ws, manifestKey, ManifestRefTypeID); err != nil {
		t.Fatal(err.Error())
	}
	got, _, err := LookupManifestRef(ctx, ws, manifestKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !got.GetReleaseAuthorization().EqualVT(ref.GetReleaseAuthorization()) {
		t.Fatal("stored manifest ref lost its release authorization")
	}
	if got.GetManifestRef().GetBucketId() == "external-bucket" {
		t.Fatal("stored manifest ref was not relocated")
	}
	collected, errs, err := CollectManifests(ctx, ws, nil, hostKey)
	if err != nil || len(errs) != 0 {
		t.Fatalf("collect manifests: %v %v", err, errs)
	}
	if got := collected["spacewave-web"]; len(got) != 1 || got[0].ReleaseAuthorization == nil {
		t.Fatalf("collected %v, want the authorized manifest", got)
	}
}

// TestExStoreManifestOpRetypesReplacedManifestRef checks that storing a
// different unauthorized manifest onto an authorized ManifestRef object
// replaces both its root and its type.
func TestExStoreManifestOpRetypesReplacedManifestRef(t *testing.T) {
	// Build a World with a manifest store.
	const hostKey, manifestKey = "release", "release/spacewave-web"
	ctx, tb, ws := newReleaseTestWorld(t, hostKey)

	// Store an authorized manifest, then a different unauthorized one.
	authorized := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 3)
	key, _, err := crypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := authorized.SignReleaseAuthorization(key); err != nil {
		t.Fatal(err.Error())
	}
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey}, authorized); err != nil {
		t.Fatal(err.Error())
	}
	next := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 4)
	if err := ExStoreManifestOp(ctx, ws, peer.ID("test"), manifestKey, []string{hostKey}, next); err != nil {
		t.Fatal(err.Error())
	}

	// The object is a Manifest whose type and root agree.
	if err := world_types.CheckObjectType(ctx, ws, manifestKey, ManifestTypeID); err != nil {
		t.Fatal(err.Error())
	}
	m, _, err := LookupManifest(ctx, ws, manifestKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if m.GetMeta().GetRev() != 4 {
		t.Fatalf("stored manifest rev %d, want 4", m.GetMeta().GetRev())
	}
	collected, errs, err := CollectManifests(ctx, ws, nil, hostKey)
	if err != nil || len(errs) != 0 {
		t.Fatalf("collect manifests: %v %v", err, errs)
	}
	if got := collected["spacewave-web"]; len(got) != 1 || got[0].ReleaseAuthorization != nil {
		t.Fatalf("collected %v, want one unauthorized manifest", got)
	}
}

// opCountingWorldState counts the operations applied to a WorldState.
type opCountingWorldState struct {
	world.WorldState
	// applied is the number of ApplyWorldOp calls.
	applied int
}

// ApplyWorldOp counts op and applies it to the wrapped WorldState.
func (w *opCountingWorldState) ApplyWorldOp(ctx context.Context, op world.Operation, sender peer.ID) (uint64, bool, error) {
	w.applied++
	return w.WorldState.ApplyWorldOp(ctx, op, sender)
}
func TestSetManifestBucketRelocationDoesNotBumpLinkedRev(t *testing.T) {

	// Set up the test context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newManifestTestWorld(t, ctx, le)
	defer releaseWorldStorage()

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

	// Start storage and an empty cursor for the manifest World.
	tb, ocs, releaseWorldStorage := newManifestTestWorld(t, ctx, le)
	defer releaseWorldStorage()

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
