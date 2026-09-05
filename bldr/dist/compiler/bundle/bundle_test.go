package dist_compiler_bundle_test

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/go-git/go-billy/v6/osfs"
	dist_compiler_bundle "github.com/s4wave/spacewave/bldr/dist/compiler/bundle"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	"github.com/s4wave/spacewave/db/block"
	block_store_kvfile "github.com/s4wave/spacewave/db/block/store/kvfile"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

func TestBundleManifestsKvfileWorldRootLifetime(t *testing.T) {
	ctx := t.Context()
	log := logrus.New()
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)

	baseCursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(baseCursor.Release)

	ocs := bucket_lookup.NewCursor(
		ctx,
		tb.Bus,
		le,
		tb.StepFactorySet,
		baseCursor.GetBucket(),
		passthroughTransform{},
		baseCursor.GetRef(),
		baseCursor.GetOpArgs(),
		nil,
	)
	eng, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(func() {
		if err := eng.Close(); err != nil {
			t.Fatal(err.Error())
		}
	})

	assetData := bytes.Repeat([]byte("packaged asset contents"), 16*1024)
	assetDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(assetDir, "asset.bin"), assetData, 0o644); err != nil {
		t.Fatal(err)
	}
	btx, bcs := ocs.BuildTransactionAtRef(nil, nil)
	_, err = bldr_manifest.CreateManifestWithBilly(ctx, bcs, bldr_manifest.NewManifestMeta("fixture", bldr_manifest.BuildType_DEV, "js", 1), "", nil, osfs.New(assetDir), timestamp.New(time.Unix(1700000000, 0)))
	if err != nil {
		t.Fatal(err)
	}
	manifestRoot, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	manifestRef := ocs.GetRef().Clone()
	manifestRef.RootRef = manifestRoot

	wtx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wtx.CreateObject(ctx, "bundle-root-closure-object", &bucket.ObjectRef{BucketId: tb.BucketId}); err != nil {
		wtx.Discard()
		t.Fatal(err.Error())
	}
	if _, err := wtx.CreateObject(ctx, "manifest-fixture", manifestRef); err != nil {
		t.Fatal(err)
	}
	if err := world_types.SetObjectType(ctx, wtx, "manifest-fixture", bldr_manifest_world.ManifestTypeID); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	rootRef := eng.GetRootRef().GetRootRef()
	if rootRef == nil || rootRef.GetEmpty() {
		t.Fatal("test setup did not create a non-empty world root ref")
	}
	rootRefStr := rootRef.MarshalString()

	kvtxVol, ok := tb.Volume.(volume_kvtx.KvtxVolume)
	if !ok {
		t.Fatalf("testbed volume type %T does not expose a kvtx store", tb.Volume)
	}

	t.Run("live world root", func(t *testing.T) {
		var buf bytes.Buffer
		kvfileWriter := kvfile.NewWriter(&buf)
		err := dist_compiler_bundle.BundleManifestsKvfile(
			ctx,
			le,
			kvfileWriter,
			store_kvkey.NewDefaultKVKey().GetBlockFullPrefix(),
			eng,
		)
		if err != nil {
			t.Fatal(err.Error())
		}
		if err := kvfileWriter.Close(); err != nil {
			t.Fatal(err.Error())
		}
		if buf.Len() == 0 {
			t.Fatal("BundleManifestsKvfile wrote an empty kvfile")
		}

		// Reopen the asset using only the packed bytes, without the source store.
		rdr, err := kvfile.BuildReader(bytes.NewReader(buf.Bytes()), uint64(buf.Len()))
		if err != nil {
			t.Fatal(err)
		}
		store := block_store_kvfile.NewKvfileBlock(ctx, store_kvkey.NewDefaultKVKey(), rdr)
		_, packedCursor := block.NewTransaction(store, nil, manifestRoot, nil)
		manifest, err := bldr_manifest.UnmarshalManifest(ctx, packedCursor)
		if err != nil {
			t.Fatal(err)
		}
		tree, err := unixfs_block.NewFSTree(ctx, manifest.FollowAssetsFsRef(packedCursor), unixfs_block.NodeType_NodeType_DIRECTORY)
		if err != nil {
			t.Fatal(err)
		}
		assetTree, _, err := tree.FollowDirent(0)
		if err != nil {
			t.Fatal(err)
		}
		handle, err := assetTree.BuildFileHandle(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer handle.Close()
		got, err := io.ReadAll(handle)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, assetData) {
			t.Fatal("packaged asset differs from input")
		}

		// Unreachable blocks from earlier builds must not affect this snapshot.
		orphan := []byte("unreachable earlier build")
		orphanRef, err := block.BuildBlockRef(orphan, nil)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := kvtxVol.GetKvtxStore().NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		orphanKey := append(bytes.Clone(kvtxVol.GetKvKey().GetBlockFullPrefix()), []byte(orphanRef.MarshalString())...)
		if err := tx.Set(ctx, orphanKey, orphan); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		var repeated bytes.Buffer
		repeatedWriter := kvfile.NewWriter(&repeated)
		if err := dist_compiler_bundle.BundleManifestsKvfile(ctx, le, repeatedWriter, store_kvkey.NewDefaultKVKey().GetBlockFullPrefix(), eng); err != nil {
			t.Fatal(err)
		}
		if err := repeatedWriter.Close(); err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(buf.Bytes(), repeated.Bytes()) {
			t.Fatalf("unchanged snapshot changed after unrelated history: %d -> %d bytes", buf.Len(), repeated.Len())
		}
	})

	t.Run("missing world root", func(t *testing.T) {
		if err := ocs.GetBucket().RmBlock(ctx, rootRef); err != nil {
			t.Fatal(err)
		}
		var buf bytes.Buffer
		kvfileWriter := kvfile.NewWriter(&buf)
		t.Cleanup(func() { _ = kvfileWriter.Close() })

		err := dist_compiler_bundle.BundleManifestsKvfile(
			ctx,
			le,
			kvfileWriter,
			store_kvkey.NewDefaultKVKey().GetBlockFullPrefix(),
			eng,
		)
		if !errors.Is(err, block.ErrNotFound) {
			t.Fatalf("BundleManifestsKvfile error = %v, want block.ErrNotFound", err)
		}
		if !strings.Contains(err.Error(), rootRefStr) {
			t.Fatalf("BundleManifestsKvfile error = %v, want missing root %s", err, rootRefStr)
		}
	})
}

type passthroughTransform struct{}

func (passthroughTransform) EncodeBlock(data []byte) ([]byte, error) {
	return data, nil
}

func (passthroughTransform) DecodeBlock(data []byte) ([]byte, error) {
	return data, nil
}
