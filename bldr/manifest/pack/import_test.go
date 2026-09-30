package bldr_manifest_pack

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

func TestImportManifestPackReconstructsCollectableManifest(t *testing.T) {
	// Import a pack into a fresh world and verify its manifests.
	ctx := context.Background()
	dest, meta, tuple := importTestManifestPack(t, ctx)
	if err := VerifyImportedManifests(ctx, dest, meta); err != nil {
		t.Fatal(err)
	}

	// Expect one collectable manifest for the tuple.
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

func TestNewPackfileStoreServesManifestBundleBlock(t *testing.T) {
	// Store a manifest bundle and pack its bytes.
	ctx := context.Background()
	source := newTestWorld(t, ctx, logrus.NewEntry(logrus.New()))
	sender := peer.ID("sender")
	tuple := testManifestPackTuple()
	manifestRef := storeTestManifest(t, ctx, source, tuple, false, true)
	_, bundleRef, err := StoreManifestBundle(ctx, source, sender, tuple, manifestRef, timestamppb.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Pack the bundle into a byte buffer.
	var buf bytes.Buffer
	entry, packDigest, err := PackManifestBundle(ctx, source, "ci-release", bundleRef, &buf)
	if err != nil {
		t.Fatal(err)
	}

	// Build the pack metadata over the packed bytes.
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

	// Open the packfile store over an in-memory transport.
	transport := manifestPackBytesTransport{data: buf.Bytes()}
	opener := func(packID string, size int64) (*packfile_store.PackReader, error) {
		return packfile_store.NewPackReader(packID, size, transport), nil
	}
	store, err := NewPackfileStore(ctx, meta, opener, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Expect the store to serve the bundle root block.
	got, found, err := store.GetBlock(ctx, bundleRef.GetRootRef())
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("manifest bundle root not found")
	}
	if len(got) == 0 {
		t.Fatal("manifest bundle root block is empty")
	}
}

type manifestPackBytesTransport struct {
	data []byte
}

func (t manifestPackBytesTransport) Fetch(_ context.Context, off int64, length int) ([]byte, error) {
	if off >= int64(len(t.data)) {
		return nil, io.EOF
	}
	end := min(off+int64(length), int64(len(t.data)))
	return bytes.Clone(t.data[off:end]), nil
}

func TestImportManifestPackIncludesBucketScopedManifestRoot(t *testing.T) {
	// Import a bucket-scoped manifest pack and verify its manifests.
	ctx := context.Background()
	dest, meta, tuple := importTestManifestPackWithOptions(t, ctx, true, false)
	if err := VerifyImportedManifests(ctx, dest, meta); err != nil {
		t.Fatal(err)
	}

	// Expect one collectable manifest for the tuple.
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

func TestImportManifestPackIncludesManifestFileTrees(t *testing.T) {
	// Import a pack with dist content and verify its manifests.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	dest, meta, tuple := importTestManifestPackWithOptions(t, ctx, false, true)
	if err := VerifyImportedManifests(ctx, dest, meta); err != nil {
		t.Fatal(err)
	}

	// Expect one collectable manifest for the tuple.
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

	// Access the imported manifest and resolve its entrypoint.
	err = bldr_manifest_world.AccessManifest(ctx, le, dest.AccessWorldState, got[0].ManifestRef, func(
		ctx context.Context,
		bls *bucket_lookup.Cursor,
		bcs *block.Cursor,
		manifest *bldr_manifest.Manifest,
		distFS *unixfs.FSHandle,
		assetsFS *unixfs.FSHandle,
	) error {
		_, _, err := distFS.LookupPath(ctx, manifest.GetEntrypoint())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestStoreManifestBundleCreatesMissingLinkManifestStore(t *testing.T) {
	// Store a manifest in a fresh world without a link store.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	sender := peer.ID("test")
	source := newTestWorld(t, ctx, le)
	tuple := testManifestPackTuple()
	manifestRef := storeTestManifest(t, ctx, source, tuple, false, false)

	// Store the bundle and expect the link manifest store to exist.
	if _, _, err := StoreManifestBundle(ctx, source, sender, tuple, manifestRef, timestamppb.Now()); err != nil {
		t.Fatal(err)
	}
	if err := bldr_manifest_world.CheckManifestStoreType(ctx, source, tuple.GetLinkObjectKeys()[0]); err != nil {
		t.Fatal(err)
	}

	// Expect one collectable manifest for the tuple.
	got, errs, err := bldr_manifest_world.CollectManifestsForManifestID(
		ctx,
		source,
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

func TestImportManifestPackCreatesMissingLinkManifestStore(t *testing.T) {
	// Import a pack into a fresh world and check the link store type.
	ctx := context.Background()
	dest, _, tuple := importTestManifestPackWithOptions(t, ctx, false, false)

	if err := bldr_manifest_world.CheckManifestStoreType(ctx, dest, tuple.GetLinkObjectKeys()[0]); err != nil {
		t.Fatal(err)
	}
}

func TestVerifyImportedManifestsRejectsWrongPlatform(t *testing.T) {
	// Import a pack and rewrite its tuple's platform ID.
	ctx := context.Background()
	dest, meta, _ := importTestManifestPack(t, ctx)
	meta = meta.CloneVT()
	meta.Manifests[0].PlatformId = "desktop/linux/amd64"

	// Expect verification to reject the mismatched platform.
	err := VerifyImportedManifests(ctx, dest, meta)
	if err == nil {
		t.Fatal("VerifyImportedManifests accepted wrong platform")
	}
	if !strings.Contains(err.Error(), "platform_id mismatch") {
		t.Fatalf("VerifyImportedManifests error = %v", err)
	}
}

func TestVerifyImportedManifestsRejectsMissingImport(t *testing.T) {
	// Verify metadata against a world that never imported the pack.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	dest := newTestWorld(t, ctx, le)
	meta := testManifestPackMetadata(t)

	// Expect verification to reject the missing import.
	err := VerifyImportedManifests(ctx, dest, meta)
	if err == nil {
		t.Fatal("VerifyImportedManifests accepted missing import")
	}
}

func TestVerifyImportedManifestsRejectsSkippedNormalManifest(t *testing.T) {
	// Import a pack and add a corrupted manifest under the link key.
	ctx := context.Background()
	dest, meta, tuple := importTestManifestPack(t, ctx)

	// Store the corrupted manifest and link it to the tuple's object key.
	badRef := storeTestManifest(t, ctx, dest, tuple, false, false).GetManifestRef().CloneVT()
	badRef.RootRef.Hash.Hash[0] ^= 0xff
	const badManifestKey = "ci/manifest-pack/bad"
	if _, _, err := bldr_manifest_world.SetManifest(ctx, dest, peer.ID("test"), badManifestKey, badRef); err != nil {
		t.Fatal(err)
	}
	if err := dest.SetGraphQuad(ctx, bldr_manifest_world.NewManifestQuad(tuple.GetLinkObjectKeys()[0], badManifestKey, tuple.GetManifestId())); err != nil {
		t.Fatal(err)
	}

	// Expect verification to reject the skipped manifest by key.
	err := VerifyImportedManifests(ctx, dest, meta)
	if err == nil {
		t.Fatal("VerifyImportedManifests accepted skipped normal manifest")
	}
	if !strings.Contains(err.Error(), "had skipped manifests") {
		t.Fatalf("VerifyImportedManifests error = %v", err)
	}
	if !strings.Contains(err.Error(), badManifestKey) {
		t.Fatalf("VerifyImportedManifests error %q does not mention bad manifest key %q", err.Error(), badManifestKey)
	}
}

func TestImportManifestPackRejectsCorruptPackDigest(t *testing.T) {
	// Import bytes that cannot match the metadata digest.
	meta := testManifestPackMetadata(t)

	// Expect the import to fail with a size mismatch.
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
	// Delegate to the option-aware helper with defaults.
	t.Helper()
	return importTestManifestPackWithOptions(t, ctx, false, false)
}

func importTestManifestPackWithOptions(
	t *testing.T,
	ctx context.Context,
	bucketScopedManifest bool,
	withDist bool,
) (world.WorldState, *ManifestPackMetadata, *ManifestTuple) {
	// Attribute test failures to the caller.
	t.Helper()

	// Build the source and destination worlds.
	le := logrus.NewEntry(logrus.New())
	sender := peer.ID("test")
	source := newTestWorld(t, ctx, le)
	dest := newTestWorld(t, ctx, le)
	tuple := testManifestPackTuple()

	// Store the source manifest and bundle it.
	manifestRef := storeTestManifest(t, ctx, source, tuple, bucketScopedManifest, withDist)
	_, bundleRef, err := StoreManifestBundle(ctx, source, sender, tuple, manifestRef, timestamppb.Now())
	if err != nil {
		t.Fatal(err)
	}

	// Pack the bundle and build the pack metadata.
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

	// Import the pack into the destination world.
	if err := ImportManifestPack(ctx, dest, sender, meta, buf.Bytes()); err != nil {
		t.Fatal(err)
	}
	return dest, meta, tuple
}

func testManifestPackTuple() *ManifestTuple {
	// Return the fixture manifest tuple.
	return &ManifestTuple{
		ManifestId:     "spacewave-web",
		PlatformId:     "js",
		Rev:            7,
		ObjectKey:      "ci/manifest-pack/spacewave-web/js",
		LinkObjectKeys: []string{"ci/manifest-pack"},
	}
}

func newTestWorld(
	t *testing.T,
	ctx context.Context,
	le *logrus.Entry,
) world.WorldState {
	// Build a testbed and an empty bucket cursor for the world.
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

	// Build the mock world state over the cursor.
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
	bucketScopedManifest bool,
	withDist bool,
) *bldr_manifest.ManifestRef {
	// Attribute test failures to the caller.
	t.Helper()

	// Build the manifest metadata from the tuple.
	meta := &bldr_manifest.ManifestMeta{
		ManifestId: tuple.GetManifestId(),
		BuildType:  "production",
		PlatformId: tuple.GetPlatformId(),
		Rev:        tuple.GetRev(),
	}

	// Scope the manifest to the bucket when requested.
	var initRef *bucket.ObjectRef
	if bucketScopedManifest {
		err := ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
			initRef = bls.GetRef().Clone()
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	// Access the object and write the manifest into it.
	entrypoint := "entrypoint"
	manifestRef, err := world.AccessObject(ctx, ws.AccessWorldState, initRef, func(bcs *block.Cursor) error {
		// Write the bare manifest without dist content.
		if !withDist {
			bcs.SetBlock(bldr_manifest.NewManifest(meta, entrypoint), true)
			return nil
		}

		// Write the entrypoint file into an in-memory dist filesystem.
		distFS := memfs.New()
		f, err := distFS.Create(entrypoint)
		if err != nil {
			return err
		}
		if _, err := f.Write([]byte("console.log('packed')\n")); err != nil {
			return err
		}
		if err := f.Close(); err != nil {
			return err
		}
		err = bldr_manifest.CreateManifestWithBilly(ctx, bcs, bldr_manifest.NewManifest(meta, entrypoint), distFS, nil, timestamppb.Now())
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return bldr_manifest.NewManifestRef(meta, manifestRef)
}
