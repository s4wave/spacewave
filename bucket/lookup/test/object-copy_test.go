package bucket_lookup_test_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_mock "github.com/aperturerobotics/hydra/block/mock"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_lz4 "github.com/aperturerobotics/hydra/block/transform/lz4"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestCopyObjectToBucket tests copying an object between buckets.
func TestCopyObjectToBucket(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	baseSrcCursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer baseSrcCursor.Release()

	srcCursor, err := baseSrcCursor.FollowRef(ctx, &bucket.ObjectRef{
		TransformConf: transformConf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer srcCursor.Release()

	// Note: a better test would be a set of blocks with BlockRefs between.
	btx, bcs := srcCursor.BuildTransaction(nil)
	rootBlk := &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{}}
	bcs.SetBlock(rootBlk, true)

	subBcs := bcs.FollowSubBlock(1)
	refBcs := subBcs.FollowRef(1, nil)
	exampleBlk := block_mock.NewExample("test block")
	refBcs.SetBlock(exampleBlk, true)

	srcRef, _, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor.SetRootRef(srcRef)

	// Set a destination transform conf
	destTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_lz4.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	destCursor, err := baseSrcCursor.FollowRef(ctx, &bucket.ObjectRef{
		TransformConf: destTransformConf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	outRef, err := bucket_lookup.CopyObjectToBucket(ctx, destCursor, srcCursor, block_mock.NewRootBlock)
	if err != nil {
		t.Fatal(err.Error())
	}

	resultCursor, err := baseSrcCursor.FollowRef(ctx, outRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resultCursor.Release()

	_, bcs = resultCursor.BuildTransaction(nil)
	outRootBlk, err := bcs.Unmarshal(block_mock.NewRootBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	outRoot := outRootBlk.(*block_mock.Root)
	if !outRoot.EqualVT(rootBlk) {
		t.FailNow()
	}

	outExampleBlk, err := bcs.
		FollowSubBlock(1).
		FollowRef(1, outRoot.GetExampleSubBlock().GetExamplePtr()).
		Unmarshal(block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	outExample := outExampleBlk.(*block_mock.Example)
	if !outExample.EqualVT(exampleBlk) {
		t.FailNow()
	}

	le.Infof("copied block graph successfully: %s", outRef.MarshalString())
}
