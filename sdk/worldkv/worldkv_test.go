package worldkv_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
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

func TestStoreWatchLiveChange(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30000000000)
	defer cancel()
	store, cleanup := openStore(t, ctx)
	defer cleanup()

	var mu sync.Mutex
	var snapshots [][]kvtx.WatchEntry
	cancelWatch, err := store.Watch(ctx, "watch/", func(entries []kvtx.WatchEntry) {
		mu.Lock()
		snapshots = append(snapshots, entries)
		mu.Unlock()
	})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer cancelWatch()

	if err := store.Put(ctx, "watch/a", []byte(`1`)); err != nil {
		t.Fatal(err.Error())
	}
	time.Sleep(200 * time.Millisecond)
	if err := store.Put(ctx, "watch/b", []byte(`2`)); err != nil {
		t.Fatal(err.Error())
	}
	time.Sleep(200 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(snapshots) < 2 {
		t.Fatalf("got %d snapshots, want >= 2 (initial + changed)", len(snapshots))
	}
	last := snapshots[len(snapshots)-1]
	if len(last) != 2 {
		t.Fatalf("last snapshot has %d entries, want 2", len(last))
	}
}

func TestStoreDeleteMany(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30000000000)
	defer cancel()
	store, cleanup := openStore(t, ctx)
	defer cleanup()

	if err := store.PutMany(ctx, map[string][]byte{
		"del/1": []byte("v1"),
		"del/2": []byte("v2"),
		"del/3": []byte("v3"),
		"keep":  []byte("kept"),
	}); err != nil {
		t.Fatal(err.Error())
	}
	if err := store.DeleteMany(ctx, []string{"del/1", "del/2"}); err != nil {
		t.Fatal(err.Error())
	}
	for _, key := range []string{"del/1", "del/2"} {
		if _, found, _ := store.Get(ctx, key); found {
			t.Fatalf("%s should be deleted", key)
		}
	}
	if _, found, _ := store.Get(ctx, "keep"); !found {
		t.Fatal("keep should not be deleted")
	}
}

func TestStoreExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30000000000)
	defer cancel()
	store, cleanup := openStore(t, ctx)
	defer cleanup()

	if err := store.Put(ctx, "exists/test", []byte("val")); err != nil {
		t.Fatal(err.Error())
	}
	exists, err := store.Exists(ctx, "exists/test")
	if err != nil || !exists {
		t.Fatalf("Exists = %v %v; want true nil", exists, err)
	}
	exists, err = store.Exists(ctx, "no/such/key")
	if err != nil || exists {
		t.Fatalf("Exists missing = %v %v; want false nil", exists, err)
	}
}
