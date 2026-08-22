package worldkv_test

import (
	"context"
	"testing"

	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/s4wave/spacewave/sdk/worldkv"
)

func openStore(t *testing.T, ctx context.Context) (*worldkv.Store, func()) {
	t.Helper()
	tb, err := world_testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	store, err := worldkv.Open(ctx, nil, tb.WorldState, "kv/test-store")
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}
	return store, tb.Release
}

func TestStorePutGetDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30000000000)
	defer cancel()
	store, cleanup := openStore(t, ctx)
	defer cleanup()

	if err := store.Put(ctx, "user/1", []byte(`{"name":"ada"}`)); err != nil {
		t.Fatal(err.Error())
	}
	got, found, err := store.Get(ctx, "user/1")
	if err != nil {
		t.Fatal(err.Error())
	}
	if !found || string(got) != `{"name":"ada"}` {
		t.Fatalf("Get = %q %v", string(got), found)
	}

	if err := store.Delete(ctx, "user/1"); err != nil {
		t.Fatal(err.Error())
	}
	if _, found, _ := store.Get(ctx, "user/1"); found {
		t.Fatal("expected deletion")
	}
}

func TestStoreScan(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30000000000)
	defer cancel()
	store, cleanup := openStore(t, ctx)
	defer cleanup()

	if err := store.Put(ctx, "task/a", []byte(`one`)); err != nil {
		t.Fatal(err.Error())
	}
	if err := store.PutMany(ctx, map[string][]byte{
		"task/b": []byte(`two`),
		"task/c": []byte(`three`),
		"other":  []byte(`x`),
	}); err != nil {
		t.Fatal(err.Error())
	}

	entries, err := store.List(ctx, "task/")
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(entries) != 3 {
		t.Fatalf("List(task/) = %d entries, want 3", len(entries))
	}
}
