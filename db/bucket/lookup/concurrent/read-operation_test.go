package lookup_concurrent

import (
	"testing"

	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/kvtx/hashmap"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	"github.com/sirupsen/logrus"
)

// TestReadOperationReleasesLocalTransaction checks the native scope boundary.
func TestReadOperationReleasesLocalTransaction(t *testing.T) {
	// Build a kvtx block store holding one retained block.
	ctx := t.Context()
	blocks := block_store_kvtx.NewKVTxBlock(
		store_kvkey.NewDefaultKVKey(),
		hashmap.NewHashmapKvtx(hashmap.NewHashmap[[]byte]()), 0, false,
	)
	ref, _, err := blocks.PutBlock(ctx, []byte("retained"), nil)
	if err != nil {
		t.Fatal(err)
	}
	conf := &bucket.Config{Id: "scoped-lookup"}
	controller := NewLookupController(logrus.NewEntry(logrus.New()), nil, &Config{
		BucketConf:       conf,
		PutBlockBehavior: PutBlockBehavior_PutBlockBehavior_ALL,
	})
	controller.PushBucketHandles(ctx, []bucket.BucketHandle{bucket.NewBucketHandle(
		conf.Id, &readScopeBucket{StoreOps: blocks, conf: conf},
	)})

	// A real native transaction serves repeated lookups until the scope closes.
	scoped, release, err := controller.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for range 2 {
		data, found, err := scoped.LookupBlock(ctx, ref)
		if err != nil || !found || string(data) != "retained" {
			t.Fatalf("scoped lookup: data=%q found=%t err=%v", data, found, err)
		}
	}
	if _, _, err := scoped.PutBlock(ctx, []byte("blocked"), nil); err != block_store.ErrReadOnly {
		t.Fatalf("scoped write: %v", err)
	}
	release()
	if _, _, err := scoped.LookupBlock(ctx, ref); err != block_store_kvtx.ErrReadOperationClosed {
		t.Fatalf("lookup after scope release: %v", err)
	}

	// Closing the read transaction leaves the original lookup writable.
	refs, _, err := controller.PutBlock(ctx, []byte("after"), nil)
	if err != nil || len(refs) != 1 {
		t.Fatalf("write after scope release: refs=%v err=%v", refs, err)
	}
	data, found, err := controller.LookupBlock(ctx, refs[0].RootRef)
	if err != nil || !found || string(data) != "after" {
		t.Fatalf("original lookup: data=%q found=%t err=%v", data, found, err)
	}
}
