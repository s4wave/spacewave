package bucket_lookup

import (
	"bytes"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	block_store_inmem "github.com/s4wave/spacewave/db/block/store/inmem"
	"github.com/s4wave/spacewave/db/bucket"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/sirupsen/logrus"
)

func TestCursorStagingCoversDirectAndTransactionStorePaths(t *testing.T) {
	// Construct a cursor whose effective block store buffers destination writes.
	ctx := t.Context()
	raw := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, true)
	stage := block.NewBufferedStore(ctx, raw)
	cursor := NewCursor(ctx, nil, logrus.NewEntry(logrus.New()), nil, raw, nil, &bucket.ObjectRef{BucketId: "staged"}, &bucket.BucketOpArgs{BucketId: "staged", VolumeId: "volume"}, nil)
	cursor.SetTransactionStore(stage)

	// Clone the staged cursor and release the original cursor.
	clone := cursor.Clone()
	cursor.Release()
	defer clone.Release()

	// Verify the clone retains separate bucket identity and staged storage.
	if clone.GetBucket() != raw || clone.GetBlockStore() != stage {
		t.Fatal("identity and effective store were conflated")
	}

	// Write a block directly through the staged cursor.
	data := []byte("direct staged block")
	ref, _, err := clone.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Verify the staged block is readable before reaching the raw store.
	got, found, err := clone.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("read: %v %v", found, err)
	}
	if found, err := raw.GetBlockExists(ctx, ref); err != nil || found {
		t.Fatalf("direct write escaped staging: %v %v", found, err)
	}

	// Write an example through a staged block transaction.
	tx, cs := clone.BuildTransactionAtRef(nil, nil)
	cs.SetBlock(block_mock.NewExample("transaction staged"), true)
	root, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Verify transaction writes remain absent from the raw store.
	if found, err := raw.GetBlockExists(ctx, root); err != nil || found {
		t.Fatalf("transaction escaped staging: %v %v", found, err)
	}

	// Publish the pending staged batch to the raw store.
	batch, err := stage.TakePending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := raw.PutBlockBatch(ctx, batch.Entries); err != nil {
		batch.Complete(err)
		t.Fatal(err)
	}
	batch.Complete(nil)

	// Verify batch publication makes the transaction root durable.
	if found, err := raw.GetBlockExists(ctx, root); err != nil || !found {
		t.Fatalf("publication missing: %v %v", found, err)
	}
}
