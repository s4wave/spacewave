package store_kvtx_badger

import (
	"bytes"
	"context"
	"errors"
	"testing"

	bdb "github.com/dgraph-io/badger/v4"
	"github.com/s4wave/spacewave/db/block"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/sirupsen/logrus"
)

// TestBadger tests all tests on top of badger.
func TestBadger(t *testing.T) {
	// Prepare the logger and key encoding for the Badger store contracts.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Open an in-memory Badger store for the storage contract suite.
	o := bdb.DefaultOptions("").WithInMemory(true)
	db, err := Open(o)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.db.Close()

	// Run the storage contracts through the Badger transaction adapter.
	ktx := store_kvtx.NewKVTx(
		kvkey,
		kvtx_vlogger.NewVLogger(le, db),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestCommitConflictIsInvalidSnapshot(t *testing.T) {
	// Open an in-memory Badger database for conflicting snapshots.
	db, err := bdb.Open(bdb.DefaultOptions("").WithInMemory(true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Commit the initial key that both Badger snapshots will observe.
	ctx := context.Background()
	seed := db.NewTransaction(true)
	if err := seed.Set([]byte("key"), []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := seed.Commit(); err != nil {
		t.Fatal(err)
	}

	// Read the key in the first Badger transaction before a competing write.
	firstStore := NewStore(db)
	firstStore.writeMtx.Lock()
	first := firstStore.newTx(db.NewTransaction(true), true)
	defer first.Discard()
	if _, _, err := first.Get(ctx, []byte("key")); err != nil {
		t.Fatal(err)
	}

	// Commit a competing Badger transaction that changes the same key.
	secondStore := NewStore(db)
	secondStore.writeMtx.Lock()
	second := secondStore.newTx(db.NewTransaction(true), true)
	if err := second.Set(ctx, []byte("key"), []byte("two")); err != nil {
		t.Fatal(err)
	}
	if err := second.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Verify the stale Badger transaction reports an invalid snapshot on commit.
	if err := first.Set(ctx, []byte("key"), []byte("three")); err != nil {
		t.Fatal(err)
	}
	err = first.Commit(ctx)
	if !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("commit error = %v, want ErrInvalidSnapshot", err)
	}
}

// TestPutBlockBatchSplitsFullTransaction writes a block batch larger than one
// Badger transaction holds and checks that every block is stored.
func TestPutBlockBatchSplitsFullTransaction(t *testing.T) {
	// Open a store whose transactions hold about 150 KiB.
	ctx := context.Background()
	db, err := Open(bdb.DefaultOptions("").WithInMemory(true).WithMemTableSize(1 << 20).WithValueThreshold(64 << 10).WithLogger(nil))
	if err != nil {
		t.Fatal(err)
	}
	defer db.db.Close()
	blocks := block_store_kvtx.NewKVTxBlock(store_kvkey.NewDefaultKVKey(), db, 0, false)

	// Write 1 MiB of distinct blocks in one batch.
	entries := make([]*block.PutBatchEntry, 64)
	for i := range entries {
		data := bytes.Repeat([]byte{byte(i)}, 16<<10)
		ref, err := block.BuildBlockRef(data, &block.PutOpts{})
		if err != nil {
			t.Fatal(err)
		}
		entries[i] = &block.PutBatchEntry{Ref: ref, Data: data}
	}
	if err := blocks.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}

	// Every block reads back.
	for i, entry := range entries {
		data, found, err := blocks.GetBlock(ctx, entry.Ref)
		if err != nil || !found || !bytes.Equal(data, entry.Data) {
			t.Fatalf("block %d: found=%v err=%v", i, found, err)
		}
	}
}
