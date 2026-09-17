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
	ctx := t.Context()
	raw := block_store_inmem.NewInmemBlock(store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), 0, true)
	stage := block.NewBufferedStore(ctx, raw)
	cursor := NewCursor(ctx, nil, logrus.NewEntry(logrus.New()), nil, raw, nil, &bucket.ObjectRef{BucketId: "staged"}, &bucket.BucketOpArgs{BucketId: "staged", VolumeId: "volume"}, nil)
	cursor.SetTransactionStore(stage)
	clone := cursor.Clone()
	cursor.Release()
	defer clone.Release()
	if clone.GetBucket() != raw || clone.GetBlockStore() != stage {
		t.Fatal("identity and effective store were conflated")
	}
	data := []byte("direct staged block")
	ref, _, err := clone.PutBlock(ctx, data, nil)
	if err != nil {
		t.Fatal(err)
	}
	got, found, err := clone.GetBlock(ctx, ref)
	if err != nil || !found || !bytes.Equal(got, data) {
		t.Fatalf("read: %v %v", found, err)
	}
	if found, err := raw.GetBlockExists(ctx, ref); err != nil || found {
		t.Fatalf("direct write escaped staging: %v %v", found, err)
	}
	tx, cs := clone.BuildTransactionAtRef(nil, nil)
	cs.SetBlock(block_mock.NewExample("transaction staged"), true)
	root, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if found, err := raw.GetBlockExists(ctx, root); err != nil || found {
		t.Fatalf("transaction escaped staging: %v %v", found, err)
	}
	batch, err := stage.TakePending(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := raw.PutBlockBatch(ctx, batch.Entries); err != nil {
		batch.Complete(err)
		t.Fatal(err)
	}
	batch.Complete(nil)
	if found, err := raw.GetBlockExists(ctx, root); err != nil || !found {
		t.Fatalf("publication missing: %v %v", found, err)
	}
}
