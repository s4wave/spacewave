package bldr_manifest_pack

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestImportManifestPackReconstructsCollectableManifest(t *testing.T) {
	ctx := context.Background()
	dest, meta, tuple := importTestManifestPack(t, ctx)
	if err := VerifyImportedManifests(ctx, dest, meta); err != nil {
		t.Fatal(err)
	}
	got, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		dest,
		tuple.GetManifestId(),
		[]string{tuple.GetPlatformId()},
		tuple.GetLinkObjectKeys()[0],
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 0 {
		t.Fatalf("manifest errors = %v", errs)
	}
	if len(got) != 1 {
		t.Fatalf("manifest count = %d", len(got))
	}
}

func TestVerifyImportedManifestsRejectsWrongPlatform(t *testing.T) {
	ctx := context.Background()
	dest, meta, _ := importTestManifestPack(t, ctx)
	meta = meta.CloneVT()
	meta.Manifests[0].PlatformId = "desktop/linux/amd64"
	err := VerifyImportedManifests(ctx, dest, meta)
	if err == nil {
		t.Fatal("VerifyImportedManifests accepted wrong platform")
	}
	if !strings.Contains(err.Error(), "platform_id mismatch") {
		t.Fatalf("VerifyImportedManifests error = %v", err)
	}
}

func TestVerifyImportedManifestsRejectsMissingImport(t *testing.T) {
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	dest := newTestWorld(t, ctx, le)
	meta := testManifestPackMetadata(t)
	err := VerifyImportedManifests(ctx, dest, meta)
	if err == nil {
		t.Fatal("VerifyImportedManifests accepted missing import")
	}
}

func TestImportManifestPackRejectsCorruptPackDigest(t *testing.T) {
	meta := testManifestPackMetadata(t)
	err := ImportManifestPack(context.Background(), nil, peer.ID("test"), meta, []byte("corrupt"))
	if err == nil {
		t.Fatal("ImportManifestPack accepted corrupt pack")
	}
	if !strings.Contains(err.Error(), "pack size mismatch") {
		t.Fatalf("ImportManifestPack error = %v", err)
	}
}

func importTestManifestPack(
	t *testing.T,
	ctx context.Context,
) (world.WorldState, *ManifestPackMetadata, *ManifestTuple) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	sender := peer.ID("test")
	source := newTestWorld(t, ctx, le)
	dest := newTestWorld(t, ctx, le)
	tuple := &ManifestTuple{
		ManifestId:     "spacewave-web",
		PlatformId:     "js",
		Rev:            7,
		ObjectKey:      "ci/manifest-pack/spacewave-web/js",
		LinkObjectKeys: []string{"ci/manifest-pack"},
	}
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, source, tuple.GetLinkObjectKeys()[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := bldr_manifest_world.CreateManifestStore(ctx, dest, tuple.GetLinkObjectKeys()[0]); err != nil {
		t.Fatal(err)
	}
	manifestRef := storeTestManifest(t, ctx, source, tuple)
	_, bundleRef, err := StoreManifestBundle(ctx, source, sender, tuple, manifestRef, timestamppb.Now())
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	entry, packDigest, err := PackManifestBundle(ctx, source, "ci-release", bundleRef, &buf)
	if err != nil {
		t.Fatal(err)
	}
	meta, err := NewMetadata(
		"0123456789abcdef0123456789abcdef01234567",
		"production",
		"spacewave-web-js",
		false,
		"manifest-pack-v1",
		[]*ManifestTuple{tuple},
		bundleRef,
		entry,
		packDigest,
	)
	if err != nil {
		t.Fatal(err)
	}

	if err := ImportManifestPack(ctx, dest, sender, meta, buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	return dest, meta, tuple
}

func newTestWorld(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
) world.WorldState {
	t.Helper()
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ocs.Release)
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		t.Fatal(err)
	}
	return ws
}

func storeTestManifest(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	tuple *ManifestTuple,
) *bldr_manifest.ManifestRef {
	t.Helper()
	meta := &bldr_manifest.ManifestMeta{
		ManifestId: tuple.GetManifestId(),
		BuildType:  "production",
		PlatformId: tuple.GetPlatformId(),
		Rev:        tuple.GetRev(),
	}
	manifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, nil, func(bcs *block.Cursor) error {
		bcs.SetBlock(bldr_manifest.NewManifest(meta, "entrypoint"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return bldr_manifest.NewManifestRef(meta, manifestRef)
}
