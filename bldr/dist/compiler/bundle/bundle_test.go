package dist_compiler_bundle_test

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/aperturerobotics/go-kvfile"
	dist_compiler_bundle "github.com/s4wave/spacewave/bldr/dist/compiler/bundle"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
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

	wtx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := wtx.CreateObject(ctx, "bundle-root-closure-object", &bucket.ObjectRef{BucketId: tb.BucketId}); err != nil {
		wtx.Discard()
		t.Fatal(err.Error())
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
			[]byte("kvfile-block/"),
			eng,
			kvtxVol.GetKvtxStore(),
			kvtxVol.GetKvKey().GetBlockFullPrefix(),
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
	})

	t.Run("missing world root", func(t *testing.T) {
		var buf bytes.Buffer
		kvfileWriter := kvfile.NewWriter(&buf)
		t.Cleanup(func() { _ = kvfileWriter.Close() })

		err := dist_compiler_bundle.BundleManifestsKvfile(
			ctx,
			le,
			kvfileWriter,
			[]byte("kvfile-block/"),
			eng,
			kvtxVol.GetKvtxStore(),
			[]byte("prefix-that-does-not-contain-blocks/"),
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
