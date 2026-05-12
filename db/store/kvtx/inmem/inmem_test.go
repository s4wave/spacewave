package store_kvtx_inmem

import (
	"bytes"
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/sirupsen/logrus"
)

// TestKVTxStore tests a key/value transaction store on top of inmem.
func TestKVTxStore(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	ktx := store_kvtx.NewKVTx(
		kvkey,
		kvtx_vlogger.NewVLogger(le, NewStore()),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestWriteIteratorSortsAddedKeys(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	for _, key := range [][]byte{
		[]byte("m"),
		[]byte("a"),
		[]byte("z"),
	} {
		if err := tx.Set(ctx, key, []byte("value")); err != nil {
			t.Fatal(err)
		}
	}

	iter := tx.Iterate(ctx, nil, true, false)
	defer iter.Close()
	if err := iter.Seek(nil); err != nil {
		t.Fatal(err)
	}
	var got [][]byte
	for iter.Valid() {
		got = append(got, bytes.Clone(iter.Key()))
		iter.Next()
	}
	if err := iter.Err(); err != nil && err != kvtx.ErrDiscarded {
		t.Fatal(err)
	}

	want := [][]byte{
		[]byte("a"),
		[]byte("m"),
		[]byte("z"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d", len(got), len(want))
	}
	for idx := range want {
		if !bytes.Equal(got[idx], want[idx]) {
			t.Fatalf("key[%d] = %q, want %q", idx, got[idx], want[idx])
		}
	}
}
