package bucket_lookup_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	transform_lz4 "github.com/s4wave/spacewave/db/block/transform/lz4"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/sirupsen/logrus"
)

// TestCopyObjectToBucket tests copying an object between buckets.
func TestCopyObjectToBucket(t *testing.T) {
	// Prepare a context and debug logger for the bucket copy test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed used by the source and destination buckets.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Configure gzip transformation for the source object graph.
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_gzip.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an empty base cursor for the source graph.
	baseSrcCursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer baseSrcCursor.Release()

	// Open the source cursor with its gzip transformation.
	srcCursor, err := baseSrcCursor.FollowRef(ctx, &bucket.ObjectRef{
		TransformConf: transformConf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer srcCursor.Release()

	// Note: a better test would be a set of blocks with BlockRefs between.
	// Create the source root and its nested example block.
	btx, bcs := srcCursor.BuildTransaction(nil)
	rootBlk := &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{}}
	bcs.SetBlock(rootBlk, true)

	// Populate the nested reference with the source example block.
	subBcs := bcs.FollowSubBlock(1)
	refBcs := subBcs.FollowRef(1, nil)
	exampleBlk := block_mock.NewExample("test block")
	refBcs.SetBlock(exampleBlk, true)

	// Write the source graph and retain its root reference.
	srcRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	srcCursor.SetRootRef(srcRef)

	// Create the destination bucket for the copied object graph.
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

	// Open the destination cursor with its LZ4 transformation.
	destCursor, err := baseSrcCursor.FollowRef(ctx, &bucket.ObjectRef{
		BucketId:      destBucketID,
		TransformConf: destTransformConf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer destCursor.Release()

	// Copy the source graph while capturing progress snapshots.
	var progress []bucket_lookup.ObjectCopyStats
	outRef, stats, err := bucket_lookup.CopyObjectToBucketWithProgress(
		ctx,
		destCursor,
		srcCursor,
		block_mock.NewRootBlock,
		-1,
		true,
		nil,
		func(current bucket_lookup.ObjectCopyStats) error {
			progress = append(progress, current)
			return nil
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify copy statistics and final progress describe the copied graph.
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
	if len(progress) == 0 {
		t.Fatal("expected live copy progress snapshots")
	}
	if last := progress[len(progress)-1]; last != stats {
		t.Fatalf("final progress = %#v, want final stats %#v", last, stats)
	}

	// Repeat the graph copy and verify existing blocks avoid additional writes.
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

	// Verify copying within the source bucket returns zero work statistics.
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

	// Open the copied object reference for graph verification.
	resultCursor, err := baseSrcCursor.FollowRef(ctx, outRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resultCursor.Release()

	// Decode the copied root and compare it with the source root.
	_, bcs = resultCursor.BuildTransaction(nil)
	outRootBlk, err := bcs.Unmarshal(ctx, block_mock.NewRootBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	outRoot := outRootBlk.(*block_mock.Root)
	if !outRoot.EqualVT(rootBlk) {
		t.FailNow()
	}

	// Decode the copied nested block and compare it with the source example.
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

	// Report the successfully verified copied object reference.
	le.Infof("copied block graph successfully: %s", outRef.MarshalString())
}

// TestCopyObjectToBucketResetsRawSourceTransformAtTransformedDestination
// verifies that a raw source copied below a transformed parent carries an
// explicit empty transform configuration for later traversals.
func TestCopyObjectToBucketResetsRawSourceTransformAtTransformedDestination(t *testing.T) {
	// Prepare the raw-source copy context and logger.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())

	// Start the storage testbed for the raw-source transformation check.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer tb.Release()

	// Open an empty base cursor for the raw source graph.
	baseCursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer baseCursor.Release()

	// Build and write the raw source root and nested example block.
	btx, bcs := baseCursor.BuildTransaction(nil)
	rootBlk := &block_mock.Root{ExampleSubBlock: &block_mock.SubBlock{}}
	bcs.SetBlock(rootBlk, true)
	exampleBlk := block_mock.NewExample("raw source")
	bcs.FollowSubBlock(1).FollowRef(1, nil).SetBlock(exampleBlk, true)
	srcRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	baseCursor.SetRootRef(srcRef)

	// Create the destination bucket and configure its S2 transformation.
	const destBucketID = "object-copy-raw-dest"
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{
		Id:  destBucketID,
		Rev: 1,
	}); err != nil {
		t.Fatal(err.Error())
	}
	destTransformConf, err := block_transform.NewConfig([]config.Config{
		&transform_s2.Config{},
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	destCursor, err := baseCursor.FollowRef(ctx, &bucket.ObjectRef{
		BucketId:      destBucketID,
		TransformConf: destTransformConf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer destCursor.Release()

	// Copy the raw source graph beneath the transformed destination.
	outRef, _, err := bucket_lookup.CopyObjectToBucketWithStats(
		ctx,
		destCursor,
		baseCursor,
		block_mock.NewRootBlock,
		-1,
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Reopen the transformed destination parent so no decoded source state can
	// mask the returned reference's inherited-transform behavior.
	reopenedParent, err := baseCursor.FollowRef(ctx, &bucket.ObjectRef{
		BucketId:      destBucketID,
		TransformConf: destTransformConf,
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer reopenedParent.Release()
	resultCursor, err := reopenedParent.FollowRef(ctx, outRef)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer resultCursor.Release()

	// Decode the copied root through the reopened transformed parent.
	_, resultBcs := resultCursor.BuildTransaction(nil)
	outRootBlk, err := resultBcs.Unmarshal(ctx, block_mock.NewRootBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !outRootBlk.(*block_mock.Root).EqualVT(rootBlk) {
		t.Fatal("reopened copied root differs from raw source root")
	}

	// Verify the copied nested block and explicit empty transformation configuration.
	outExampleBlk, err := resultBcs.
		FollowSubBlock(1).
		FollowRef(1, outRootBlk.(*block_mock.Root).GetExampleSubBlock().GetExamplePtr()).
		Unmarshal(ctx, block_mock.NewExampleBlock)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !outExampleBlk.(*block_mock.Example).EqualVT(exampleBlk) {
		t.Fatal("reopened copied nested block differs from raw source block")
	}
	if outRef.GetTransformConfRef().GetEmpty() {
		t.Fatal("copied raw object ref is missing explicit empty transform config")
	}
	if !outRef.GetTransformConf().GetEmpty() {
		t.Fatal("copied raw object ref unexpectedly carries inline transform config")
	}
}
