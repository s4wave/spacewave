package store_kvtx_badger

import (
	"context"
	"errors"
	"testing"

	bdb "github.com/dgraph-io/badger/v4"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/sirupsen/logrus"
)

// TestBadger tests all tests on top of badger.
func TestBadger(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	o := bdb.DefaultOptions("").WithInMemory(true)
	db, err := Open(o)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.db.Close()

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
	db, err := bdb.Open(bdb.DefaultOptions("").WithInMemory(true))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	seed := db.NewTransaction(true)
	if err := seed.Set([]byte("key"), []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := seed.Commit(); err != nil {
		t.Fatal(err)
	}

	firstStore := NewStore(db)
	firstStore.writeMtx.Lock()
	first := firstStore.newTx(db.NewTransaction(true), true)
	defer first.Discard()
	if _, _, err := first.Get(ctx, []byte("key")); err != nil {
		t.Fatal(err)
	}

	secondStore := NewStore(db)
	secondStore.writeMtx.Lock()
	second := secondStore.newTx(db.NewTransaction(true), true)
	if err := second.Set(ctx, []byte("key"), []byte("two")); err != nil {
		t.Fatal(err)
	}
	if err := second.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	if err := first.Set(ctx, []byte("key"), []byte("three")); err != nil {
		t.Fatal(err)
	}
	err = first.Commit(ctx)
	if !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("commit error = %v, want ErrInvalidSnapshot", err)
	}
}
