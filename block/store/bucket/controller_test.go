package block_store_bucket

import (
	"context"
	"testing"

	block_mock "github.com/aperturerobotics/hydra/block/mock"
	block_store "github.com/aperturerobotics/hydra/block/store"
	block_store_inmem "github.com/aperturerobotics/hydra/block/store/inmem"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_all "github.com/aperturerobotics/hydra/block/transform/all"
	"github.com/aperturerobotics/hydra/bucket"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/sirupsen/logrus"
)

// TestBlockStoreBucketController tests the block store bucket controller.
func TestBlockStoreBucketController(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	verbose := true
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(nil), testbed.WithVerbose(verbose))
	if err != nil {
		t.Fatal(err.Error())
	}

	// create a in memory block store
	blockStoreID := "test-block-store"
	storeCtrl := block_store_inmem.NewController(le, &block_store_inmem.Config{BlockStoreId: blockStoreID, Verbose: verbose})
	relStore, err := tb.Bus.AddController(ctx, storeCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relStore()

	bucketID := "test-block-store-bucket"
	bucketConf, err := bucket.NewConfig(bucketID, 1, nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// create a block store bucket controller
	bucketCtrl := NewController(blockStoreID, bucketConf, block_store.NewAccessBlockStoreViaBusFunc(tb.Bus, blockStoreID, false))
	relBucketCtrl, err := tb.Bus.AddController(ctx, bucketCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relBucketCtrl()

	// attempt to access the bucket
	bls, _, err := bucket_lookup.BuildEmptyCursor(
		ctx,
		tb.Bus,
		le,
		transform_all.BuildFactorySet(),
		bucketID,
		blockStoreID,
		&block_transform.Config{
			Steps: []*block_transform.StepConfig{{Id: "hydra/transform/lz4"}},
		},
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer bls.Release()

	btx, bcs := bls.BuildTransaction(nil)
	bcs.SetBlock(block_mock.NewExample("hello world"), true)

	rootRef, bcs, err := btx.Write(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Logf("wrote ref: %v", rootRef.MarshalLog())
}
