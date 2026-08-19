package spacewave_cli

import (
	"context"
	"fmt"
	"testing"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/testbed"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestCollectLatestManifestSetDeterministicPlatformsAndRevisions(t *testing.T) {
	ctx := context.Background()
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ocs.Release()
	ws, err := world_block.BuildMockWorldState(ctx, tb.Logger, true, ocs, false)
	if err != nil {
		t.Fatal(err)
	}
	const host = "devtool"
	if _, err := manifest_world.CreateManifestStore(ctx, ws, host); err != nil {
		t.Fatal(err)
	}
	refs := []*manifest.ManifestRef{
		testManifestRef(t, ctx, tb, "glados-core", "js", 3, "js"),
		testManifestRef(t, ctx, tb, "glados-core", "desktop/darwin/arm64", 2, "native"),
		testManifestRef(t, ctx, tb, "glados-core", "desktop/darwin/arm64", 1, "old"),
	}
	for i, ref := range refs {
		key := "devtool/manifest/" + string(rune('a'+i))
		if _, _, err := manifest_world.SetManifest(ctx, ws, peer.ID("test"), key, ref.GetManifestRef()); err != nil {
			t.Fatal(err)
		}
		if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad(host, key, "glados-core")); err != nil {
			t.Fatal(err)
		}
	}
	got, err := collectLatestManifestSet(ctx, ws, "glados-core")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("set length = %d, want 2", len(got))
	}
	if got[0].GetMeta().GetPlatformId() != "js" || got[0].GetMeta().GetRev() != 3 {
		t.Fatalf("first = %s/%d", got[0].GetMeta().GetPlatformId(), got[0].GetMeta().GetRev())
	}
	if got[1].GetMeta().GetPlatformId() != "desktop/darwin/arm64" || got[1].GetMeta().GetRev() != 2 {
		t.Fatalf("second = %s/%d", got[1].GetMeta().GetPlatformId(), got[1].GetMeta().GetRev())
	}
}

func TestCollectLatestManifestSetCollapsesIdenticalAndRejectsAmbiguous(t *testing.T) {
	ctx := context.Background()
	build := func(ambiguous bool) error {
		tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(logrus.New()))
		if err != nil {
			return err
		}
		defer tb.Release()
		ocs, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			return err
		}
		defer ocs.Release()
		ws, err := world_block.BuildMockWorldState(ctx, tb.Logger, true, ocs, false)
		if err != nil {
			return err
		}
		if _, err := manifest_world.CreateManifestStore(ctx, ws, "devtool"); err != nil {
			return err
		}
		ref := testManifestRef(t, ctx, tb, "glados-core", "js", 4, "js")
		refs := []*manifest.ManifestRef{ref, ref}
		if ambiguous {
			refs[1] = createTestManifestRefVariant(t, ctx, tb, "glados-core", "js", 4)
		}
		for i, item := range refs {
			key := "devtool/manifest/" + string(rune('a'+i))
			if _, _, err := manifest_world.SetManifest(ctx, ws, peer.ID("test"), key, item.GetManifestRef()); err != nil {
				return err
			}
			if err := ws.SetGraphQuad(ctx, manifest_world.NewManifestQuad("devtool", key, "glados-core")); err != nil {
				return err
			}
		}
		got, err := collectLatestManifestSet(ctx, ws, "glados-core")
		if ambiguous {
			if err == nil {
				return fmt.Errorf("ambiguous set unexpectedly succeeded")
			}
			return nil
		}
		if err != nil {
			return err
		}
		if len(got) != 1 {
			return fmt.Errorf("collapsed set length = %d", len(got))
		}
		return nil
	}
	if err := build(false); err != nil {
		t.Fatal(err)
	}
	if err := build(true); err != nil {
		t.Fatal(err)
	}
}

func createTestManifestRefVariant(t *testing.T, ctx context.Context, tb *testbed.Testbed, manifestID, platformID string, rev uint64) *manifest.ManifestRef {
	t.Helper()
	meta := &manifest.ManifestMeta{ManifestId: manifestID, BuildType: "production", PlatformId: platformID, Rev: rev}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer oc.Release()
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(manifest.NewManifest(meta, "different-entrypoint"), true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	oc.SetRootRef(rootRef)
	return manifest.NewManifestRef(meta, oc.GetRef())
}

func testManifestRef(t *testing.T, ctx context.Context, tb *testbed.Testbed, id, platform string, rev uint64, entrypoint string) *manifest.ManifestRef {
	t.Helper()
	meta := &manifest.ManifestMeta{ManifestId: id, BuildType: "production", PlatformId: platform, Rev: rev}
	oc, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer oc.Release()
	btx, bcs := oc.BuildTransaction(nil)
	bcs.SetBlock(manifest.NewManifest(meta, entrypoint), true)
	root, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	oc.SetRootRef(root)
	return manifest.NewManifestRef(meta, oc.GetRef())
}
