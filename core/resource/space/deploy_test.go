package resource_space

import (
	"context"
	"testing"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/memfs"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

func TestCopyBlockDAGWithTransformCopiesNestedUnixFSDirents(t *testing.T) {
	ctx, entrypoint, src, rootRef := createTestManifestStore(t)

	dest := block_mock.NewMockStore(0)
	if err := copyBlockDAGWithTransform(ctx, rootRef, bldr_manifest.NewManifestBlock, src, dest, nil); err != nil {
		t.Fatal(err)
	}

	assertManifestEntrypointExists(t, ctx, dest, rootRef, entrypoint)
}

func TestCopyBlockDAGWithTransformTraversesExistingBlocks(t *testing.T) {
	ctx, entrypoint, src, rootRef := createTestManifestStore(t)

	rootData, found, err := src.GetBlock(ctx, rootRef)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("root block not found")
	}

	dest := block_mock.NewMockStore(0)
	if _, _, err := dest.PutBlock(ctx, rootData, nil); err != nil {
		t.Fatal(err)
	}

	if err := copyBlockDAGWithTransform(ctx, rootRef, bldr_manifest.NewManifestBlock, src, dest, nil); err != nil {
		t.Fatal(err)
	}

	assertManifestEntrypointExists(t, ctx, dest, rootRef, entrypoint)
}

func createTestManifestStore(t *testing.T) (
	context.Context,
	string,
	block.StoreOps,
	*block.BlockRef,
) {
	t.Helper()

	ctx := context.Background()
	entrypoint := "plugin.js"

	distFS := memfs.New()
	f, err := distFS.Create(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Write([]byte("console.log('ok')\n")); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	src := block_mock.NewMockStore(0)
	tx, cursor := block.NewTransaction(src, nil, nil, nil)
	meta := bldr_manifest.NewManifestMeta("test-plugin", bldr_manifest.BuildType_DEV, "web/js", 1)
	if _, err := bldr_manifest.CreateManifestWithBilly(ctx, cursor, meta, entrypoint, distFS, nil, timestamppb.Now()); err != nil {
		t.Fatal(err)
	}
	rootRef, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	return ctx, entrypoint, src, rootRef
}

func assertManifestEntrypointExists(
	t *testing.T,
	ctx context.Context,
	dest block.StoreOps,
	rootRef *block.BlockRef,
	entrypoint string,
) {
	t.Helper()
	objRef := &bucket.ObjectRef{BucketId: "dest", RootRef: rootRef}
	bls := bucket_lookup.NewCursor(
		ctx,
		nil,
		logrus.NewEntry(logrus.New()),
		nil,
		dest,
		nil,
		objRef,
		&bucket.BucketOpArgs{BucketId: "dest"},
		nil,
	)
	defer bls.Release()

	err := bldr_manifest.AccessManifest(ctx, logrus.NewEntry(logrus.New()), bls, func(
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

func testDeployManifestRef(id, platform string, rev uint64, hashByte byte) *bldr_manifest.ManifestRef {
	meta := &bldr_manifest.ManifestMeta{ManifestId: id, BuildType: "production", PlatformId: platform, Rev: rev}
	root := block.NewBlockRef(hash.NewHash(hash.HashType_HashType_SHA256, []byte{hashByte}))
	return bldr_manifest.NewManifestRef(meta, &bucket.ObjectRef{RootRef: root})
}

func TestValidateManifestSetRejectsMixedIDsAndDuplicatePlatforms(t *testing.T) {
	if _, err := validateManifestSet([]*bldr_manifest.ManifestRef{
		testDeployManifestRef("glados-core", "js", 1, 1),
		testDeployManifestRef("other", "desktop/darwin/arm64", 1, 2),
	}); err == nil {
		t.Fatal("mixed manifest IDs accepted")
	}
	if _, err := validateManifestSet([]*bldr_manifest.ManifestRef{
		testDeployManifestRef("glados-core", "js", 1, 1),
		testDeployManifestRef("glados-core", "js", 2, 2),
	}); err == nil {
		t.Fatal("duplicate platform accepted")
	}
}

type deployTestSource struct {
	block.StoreOps
	data []byte
}

func (s *deployTestSource) GetBlock(context.Context, *block.BlockRef) ([]byte, bool, error) {
	return s.data, true, nil
}

func TestValidateBlockResponseRefRejectsMismatch(t *testing.T) {
	want := block.NewBlockRef(hash.NewHash(hash.HashType_HashType_SHA256, []byte("want")))
	got := block.NewBlockRef(hash.NewHash(hash.HashType_HashType_SHA256, []byte("got")))
	if err := validateBlockResponseRef(want, got); err == nil {
		t.Fatal("mismatched response ref accepted")
	}
	if err := validateBlockResponseRef(want, want); err != nil {
		t.Fatal(err)
	}
}

func TestCopyBlockRejectsStoredHashMismatch(t *testing.T) {
	ctx := context.Background()
	data := []byte("content")
	requested := block.NewBlockRef(hash.NewHash(hash.HashType_HashType_SHA256, []byte("requested")))
	dest := block_mock.NewMockStore(0)
	src := &deployTestSource{StoreOps: block_mock.NewMockStore(0), data: data}
	if err := copyBlockWithTransform(ctx, requested, nil, src, dest, nil, make(map[string]bool)); err == nil {
		t.Fatal("stored hash mismatch accepted")
	}
}

func TestValidateCopiedManifestRejectsMetadataMismatchAndCancellation(t *testing.T) {
	ctx := context.Background()
	meta := &bldr_manifest.ManifestMeta{ManifestId: "glados-core", BuildType: "production", PlatformId: "js", Rev: 1}
	data, err := bldr_manifest.NewManifest(meta, "entrypoint").MarshalBlock()
	if err != nil {
		t.Fatal(err)
	}
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	dest := block_mock.NewMockStore(0)
	if _, _, err := dest.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref}); err != nil {
		t.Fatal(err)
	}
	wrongMeta := meta.CloneVT()
	wrongMeta.Rev = 2
	if err := validateCopiedManifest(ctx, dest, ref, wrongMeta, nil); err == nil {
		t.Fatal("metadata mismatch accepted")
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if err := validateCopiedManifest(cancelled, dest, ref, meta, nil); err == nil {
		t.Fatal("cancelled copied-manifest validation succeeded")
	}
}
