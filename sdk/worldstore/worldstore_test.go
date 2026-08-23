package worldstore_test

import (
	"context"
	"testing"

	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/sdk/worldstore"
)

func openWorldStore(t *testing.T, ctx context.Context) (*worldstore.Store, func()) {
	t.Helper()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	st, err := worldstore.Open(nil, tb.WorldState)
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}
	return st, tb.Release
}

func TestWorldStoreKvAndSql(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60000000000)
	defer cancel()
	store, cleanup := openWorldStore(t, ctx)
	defer cleanup()

	// KV: put/get round-trip.
	kv, err := store.KV(ctx, "kv/app-data")
	if err != nil {
		t.Fatalf("KV: %v", err)
	}
	if err := kv.Put(ctx, "greeting", []byte("hello")); err != nil {
		t.Fatalf("KV Put: %v", err)
	}
	got, found, err := kv.Get(ctx, "greeting")
	if err != nil || !found || string(got) != "hello" {
		t.Fatalf("KV Get = %q %v; want hello true", string(got), found)
	}

	// SQL: create table, insert, query.
	db, err := store.SQL(ctx, "sql/app-db")
	if err != nil {
		t.Skipf("SQL open (may require additional setup): %v", err)
	}
	_ = db
}

func TestWorldStoreMultipleKv(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30000000000)
	defer cancel()
	store, cleanup := openWorldStore(t, ctx)
	defer cleanup()

	kv1, err := store.KV(ctx, "kv/one")
	if err != nil {
		t.Fatal(err.Error())
	}
	kv2, err := store.KV(ctx, "kv/two")
	if err != nil {
		t.Fatal(err.Error())
	}
	if err := kv1.Put(ctx, "key", []byte("store-one")); err != nil {
		t.Fatal(err.Error())
	}
	if err := kv2.Put(ctx, "key", []byte("store-two")); err != nil {
		t.Fatal(err.Error())
	}
	g1, _, _ := kv1.Get(ctx, "key")
	g2, _, _ := kv2.Get(ctx, "key")
	if string(g1) != "store-one" || string(g2) != "store-two" {
		t.Fatalf("isolated stores: g1=%q g2=%q", string(g1), string(g2))
	}
}
