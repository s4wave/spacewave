package object_mock

import (
	"context"
	"testing"

	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/object"
)

func TestPrefixer(t *testing.T) {
	ctx := context.Background()
	objs, _ := BuildTestStore(t)
	pf := object.NewPrefixer(objs, []byte("test-prefix/"))
	newTx := func(t *testing.T, write bool) kvtx.Tx {
		tx, err := pf.NewTransaction(ctx, write)
		if err != nil {
			t.Fatal(err.Error())
		}
		return tx
	}
	testSeq := "testing123"
	tx := newTx(t, true)
	if err := tx.Set(ctx, []byte("test"), []byte(testSeq)); err != nil {
		t.Fatal(err.Error())
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err.Error())
	}
	tx = newTx(t, false)
	val, found, err := tx.Get(ctx, []byte("test"))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found {
		t.FailNow()
	}
	if string(val) != testSeq {
		t.FailNow()
	}
	tx.Discard()
	tx = newTx(t, false)
	var keys []string
	err = tx.ScanPrefix(ctx, nil, func(key, value []byte) error {
		keys = append(keys, string(key))
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	if keys[0] != "test" {
		t.Fatalf("expected test, got %s", keys[0])
	}
	tx.Discard()

	tx = newTx(t, true)
	if err := tx.Delete(ctx, []byte("test")); err != nil {
		t.Fatal(err.Error())
	}
	tx.Discard()
}
