package bldr_manifest_world

import (
	"context"
	"strings"
	"testing"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestCollectReleaseWorldManifestsForManifestID(t *testing.T) {
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

	const releaseManifestKey = "spacewave/release/manifests"
	if _, err := CreateManifestStore(ctx, ws, releaseManifestKey); err != nil {
		t.Fatal(err.Error())
	}

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

func TestCollectStartupManifestsSkipsUnreadableLinkedRef(t *testing.T) {
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

	const storeKey = "plugin-host"
	if _, err := CreateManifestStore(ctx, ws, storeKey); err != nil {
		t.Fatal(err.Error())
	}

	goodRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 7)
	const goodRefKey = "plugin-host/ref/good"
	storeTestManifestRefObject(t, ctx, ws, goodRefKey, goodRef)
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, goodRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

	badRef := createTestManifestRef(t, ctx, tb, "spacewave-web", "js", 9).GetManifestRef().CloneVT()
	badRef.RootRef.Hash.Hash[0] ^= 0xff
	const badRefKey = "plugin-host/ref/missing"
	if _, err := ws.CreateObject(ctx, badRefKey, badRef); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(storeKey, badRefKey, "spacewave-web")); err != nil {
		t.Fatal(err.Error())
	}

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
	if len(errs) != 1 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if !strings.Contains(errs[0].Error(), badRefKey) {
		t.Fatalf("manifest error %q does not mention bad ref key %q", errs[0].Error(), badRefKey)
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

func TestCollectDirectManifestForManifestID(t *testing.T) {
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

	const manifestKey = "glados-core"
	ref := createTestManifestRef(t, ctx, tb, manifestKey, "js", 7)
	if _, _, err := SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}
	if err := ws.SetGraphQuad(ctx, NewManifestQuad(manifestKey, manifestKey, manifestKey)); err != nil {
		t.Fatal(err.Error())
	}

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

func createTestManifestRef(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	manifestID string,
	platformID string,
	rev uint64,
) *manifest.ManifestRef {
	t.Helper()

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

	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(manifest.NewManifest(meta, "entrypoint"), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(rootRef)
	return manifest.NewManifestRef(meta, oc.GetRef())
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
