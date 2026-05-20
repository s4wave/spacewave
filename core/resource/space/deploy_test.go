//go:build !goscript

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
