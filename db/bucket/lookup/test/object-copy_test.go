package bucket_lookup_test_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_lz4 "github.com/s4wave/spacewave/db/block/transform/lz4"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
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
		&transform_gzip.Config{},
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

	srcRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor.SetRootRef(srcRef)

	const destBucketID = "object-copy-dest"
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  destBucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}

	// Set a destination transform conf
	destTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_lz4.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	destCursor, err := baseSrcCursor.FollowRef(ctx, &bucket.ObjectRef{
		BucketId:      destBucketID,
		TransformConf: destTransformConf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	outRef, stats, err := bucket_lookup.CopyObjectToBucketWithStats(
		ctx,
		destCursor,
		srcCursor,
		block_mock.NewRootBlock,
		-1,
		true,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if stats.BlocksSeen == 0 {
		t.Fatal("expected copied graph to contain logical source blocks")
	}
	if stats.BlocksCopied != stats.BlocksWritten+stats.BlocksExisting {
		t.Fatalf(
			"copied blocks = %d, want written + existing = %d",
			stats.BlocksCopied,
			stats.BlocksWritten+stats.BlocksExisting,
		)
	}
	if stats.BlocksCopied == 0 || stats.LogicalSourceBytes == 0 {
		t.Fatalf("copy stats = %#v, want copied blocks and source bytes", stats)
	}
	_, repeatedStats, err := bucket_lookup.CopyObjectToBucketWithStats(
		ctx,
		destCursor,
		srcCursor,
		block_mock.NewRootBlock,
		-1,
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if repeatedStats.BlocksSeen == 0 || repeatedStats.LogicalSourceBytes == 0 {
		t.Fatalf("repeated copy stats = %#v, want logical source totals", repeatedStats)
	}
	if repeatedStats.BlocksWritten != 0 ||
		repeatedStats.BlocksExisting != repeatedStats.BlocksCopied {
		t.Fatalf("repeated copy stats = %#v, want existing destination blocks", repeatedStats)
	}

	_, sameBucketStats, err := bucket_lookup.CopyObjectToBucketWithStats(
		ctx,
		srcCursor,
		srcCursor,
		block_mock.NewRootBlock,
		-1,
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	if sameBucketStats != (bucket_lookup.ObjectCopyStats{}) {
		t.Fatalf("same bucket stats = %#v, want zero", sameBucketStats)
	}

	resultCursor, err := baseSrcCursor.FollowRef(ctx, outRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resultCursor.Release()

	_, bcs = resultCursor.BuildTransaction(nil)
	outRootBlk, err := bcs.Unmarshal(ctx, block_mock.NewRootBlock)
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
		Unmarshal(ctx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	outExample := outExampleBlk.(*block_mock.Example)
	if !outExample.EqualVT(exampleBlk) {
		t.FailNow()
	}

	le.Infof("copied block graph successfully: %s", outRef.MarshalString())
}
