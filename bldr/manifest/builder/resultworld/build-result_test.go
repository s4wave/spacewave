package resultworld

import (
	"context"
	"testing"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_builder "github.com/s4wave/spacewave/bldr/manifest/builder"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/testbed"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestManifestBuildResultRoundTrip(t *testing.T) {
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
	if _, _, err := bldr_manifest_world.SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}

	storedManifest, storedManifestRef, err := bldr_manifest_world.LookupManifest(ctx, ws, manifestKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	result := bldr_manifest_builder.NewBuilderResult(
		storedManifest,
		storedManifestRef,
		bldr_manifest_builder.NewInputManifest([]string{"go.mod"}, nil),
	)
	if _, err := SetManifestBuildResult(ctx, ws, manifestKey, result); err != nil {
		t.Fatal(err.Error())
	}

	got, gotRef, err := LookupManifestBuildResult(ctx, ws, manifestKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	if gotRef == nil {
		t.Fatal("build result ref missing")
	}
	if !got.GetManifest().EqualVT(storedManifest) {
		t.Fatal("manifest was not preserved")
	}
	if got.GetInputManifest().GetFiles()[0].GetPath() != "go.mod" {
		t.Fatalf("input path = %q", got.GetInputManifest().GetFiles()[0].GetPath())
	}
}

func TestManifestBuildResultRoundTripAllowsAssetOnlyManifest(t *testing.T) {
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

	const manifestKey = "asset-only-vite-child"
	ref := createTestManifestRef(t, ctx, tb, manifestKey, "js", 8)
	if _, _, err := bldr_manifest_world.SetManifest(ctx, ws, peer.ID("test"), manifestKey, ref.GetManifestRef()); err != nil {
		t.Fatal(err.Error())
	}

	storedManifest, storedManifestRef, err := bldr_manifest_world.LookupManifest(ctx, ws, manifestKey)
	if err != nil {
		t.Fatal(err.Error())
	}
	assetOnlyManifest := storedManifest.CloneVT()
	assetOnlyManifest.Entrypoint = ""
	result := bldr_manifest_builder.NewBuilderResult(
		assetOnlyManifest,
		storedManifestRef,
		bldr_manifest_builder.NewInputManifest([]string{"dist/assets.json"}, nil),
	)
	if _, err := SetManifestBuildResult(ctx, ws, manifestKey, result); err != nil {
		t.Fatalf("SetManifestBuildResult rejected asset-only manifest: %v", err)
	}

	got, gotRef, err := LookupManifestBuildResult(ctx, ws, manifestKey)
	if err != nil {
		t.Fatalf("LookupManifestBuildResult rejected asset-only manifest: %v", err)
	}
	if gotRef == nil {
		t.Fatal("build result ref missing")
	}
	if got.GetManifest().GetEntrypoint() != "" {
		t.Fatalf("entrypoint = %q, want empty", got.GetManifest().GetEntrypoint())
	}
	if !got.GetManifest().EqualVT(assetOnlyManifest) {
		t.Fatal("asset-only manifest was not preserved")
	}
	if got.GetInputManifest().GetFiles()[0].GetPath() != "dist/assets.json" {
		t.Fatalf("input path = %q", got.GetInputManifest().GetFiles()[0].GetPath())
	}
}

func createTestManifestRef(
	t *testing.T,
	ctx context.Context,
	tb *testbed.Testbed,
	manifestID string,
	platformID string,
	rev uint64,
) *bldr_manifest.ManifestRef {
	t.Helper()

	meta := &bldr_manifest.ManifestMeta{
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
	bcs.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	oc.SetRootRef(rootRef)
	return bldr_manifest.NewManifestRef(meta, oc.GetRef())
}
