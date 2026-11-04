package world_block

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	blockgc "github.com/s4wave/spacewave/db/block/gc"
	kvblock "github.com/s4wave/spacewave/db/kvtx/block"
	volumekvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"

	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/tx"
	volumes4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	worldmock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// setupReadCursorWorld builds a real file-backed engine with a multi-level index.
func setupReadCursorWorld(tb testing.TB, impl kvblock.KVImplType) (*Engine, func()) {
	// Keep storage, cursor and decoded retention alive until the engine closes.
	tb.Helper()
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	bed, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volumes4db.Config{Path: filepath.Join(tb.TempDir(), "world.s4wave"), SingleProcess: true, ReserveBytes: -1}))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(bed.Release)
	base, err := bed.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(base.Release)
	cache, err := block.NewDecodedBlockCacheWithOptions(block.DecodedBlockCacheOptions{MaxCost: 64 << 20})
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(cache.Close)
	base.SetDecodedBlockCache(cache)

	// Select the persisted backend explicitly without changing the product default.
	bt, cs := base.BuildTransaction(nil)
	root := NewWorld(false)
	root.ObjectKeyValue = kvblock.NewKeyValueStore(impl)
	cs.SetBlock(root, true)
	ref, _, err := bt.Write(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	base.SetRootRef(ref)

	engine, err := NewEngine(ctx, le, base, worldmock.LookupMockOp, nil, false)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() {
		if err := engine.Close(); err != nil {
			tb.Error(err)
		}
	})

	// Publish enough objects to exercise several immutable page levels.
	writer, err := engine.NewTransaction(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	defer writer.Discard()
	for i := range 4096 {
		object, err := writer.CreateObject(ctx, "reader/"+strconv.Itoa(i), nil)
		world.ReleaseObjectState(object)
		if err != nil {
			tb.Fatal(err)
		}
	}
	if err := writer.Commit(ctx); err != nil {
		tb.Fatal(err)
	}
	if _, err := engine.Sync(ctx); err != nil {
		tb.Fatal(err)
	}

	// Collect real storage and retire memory so a stale node cannot mask loss.
	collect := func() {
		// Apply released root pins before walking the persistent reference graph.
		volume := bed.Volume.(*volumekvtx.Volume)
		if err := volume.ReapRootPins(ctx); err != nil {
			tb.Fatal(err)
		}

		// Run the real volume collector while both snapshot roots remain pinned.
		if _, err := blockgc.NewCollector(volume.GetRefGraph(), volume, nil).Collect(ctx); err != nil {
			tb.Fatal(err)
		}

		// Retire decoded pages so a pointer hit cannot hide a missing stored page.
		cache.InvalidateAll(ctx)
	}
	return engine, collect
}

// TestWorldReadCursorRoots checks World object reads while publication replaces
// HEAD and an existing read transaction retains its old revision.
func TestWorldReadCursorRoots(t *testing.T) {
	for _, impl := range []kvblock.KVImplType{kvblock.KVImplType_KV_IMPL_TYPE_IAVL, kvblock.KVImplType_KV_IMPL_TYPE_OKRA} {
		t.Run(impl.String(), func(t *testing.T) {
			// Hold and warm the original snapshot through the production object interface.
			ctx := t.Context()
			engine, collect := setupReadCursorWorld(t, impl)
			old, err := engine.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer old.Discard()
			root, err := old.(*EngineTx).readTx.Load().state.GetRoot(ctx)
			if err != nil || root.GetObjectKeyValue().GetImplType() != impl {
				t.Fatalf("wrong persisted backend: %v", err)
			}

			for i := range 64 {
				object, found, err := old.GetObject(ctx, "reader/"+strconv.Itoa(i*61))
				world.ReleaseObjectState(object)
				if err != nil || !found {
					t.Fatalf("original object found=%v err=%v", found, err)
				}
			}

			// A rejected graph writer cannot leave deferred work in a snapshot.
			called := false
			err = old.AccessCayleyGraph(ctx, true, func(context.Context, world.CayleyHandle) error {
				called = true
				return nil
			})
			if !errors.Is(err, tx.ErrNotWrite) || called {
				t.Fatalf("readonly graph writer called=%v err=%v", called, err)
			}
			fork, err := old.(*EngineTx).Fork(ctx)
			if err != nil {
				t.Fatalf("readonly fork after graph refusal: %v", err)
			}
			fork.(*Tx).Discard()

			// Publish a newly inserted object while the warmed old root remains held.
			writer, err := engine.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer writer.Discard()
			object, err := writer.CreateObject(ctx, "reader/new", nil)
			world.ReleaseObjectState(object)
			if err != nil {
				t.Fatal(err)
			}
			if err := writer.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			fresh, err := engine.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer fresh.Discard()

			// Collection must preserve both roots even after decoded memory retires.
			collect()
			for _, snapshot := range []world.Tx{old, fresh} {
				for i := range 64 {
					object, found, err := snapshot.GetObject(ctx, "reader/"+strconv.Itoa(i*61))
					world.ReleaseObjectState(object)
					if err != nil || !found {
						t.Fatalf("collection lost object: found=%v err=%v", found, err)
					}
				}
			}

			// Require each revision to expose only the objects it actually contains.
			object, found, err := fresh.GetObject(ctx, "reader/new")
			world.ReleaseObjectState(object)
			if err != nil || !found {
				t.Fatalf("new snapshot found=%v err=%v", found, err)
			}
			object, found, err = old.GetObject(ctx, "reader/new")
			world.ReleaseObjectState(object)
			if err != nil || found {
				t.Fatalf("old snapshot advanced found=%v err=%v", found, err)
			}
		})
	}
}

// BenchmarkWorldReadCursor measures opening a snapshot and reading object
// metadata through Engine, World, Okra, bucket and file-backed volume paths.
func BenchmarkWorldReadCursor(b *testing.B) {
	for _, impl := range []kvblock.KVImplType{kvblock.KVImplType_KV_IMPL_TYPE_IAVL, kvblock.KVImplType_KV_IMPL_TYPE_OKRA} {
		b.Run(impl.String(), func(b *testing.B) {
			// Warm the fixed query set before measuring complete read transactions.
			engine, _ := setupReadCursorWorld(b, impl)
			ctx := context.Background()
			keys := make([]string, 64)
			for i := range keys {
				keys[i] = "reader/" + strconv.Itoa(i*61)
			}
			read := func() {
				tx, err := engine.NewTransaction(ctx, false)
				if err != nil {
					b.Fatal(err)
				}
				defer tx.Discard()
				for _, key := range keys {
					object, found, err := tx.GetObject(ctx, key)
					if err != nil || !found {
						world.ReleaseObjectState(object)
						b.Fatalf("object %s found=%v err=%v", key, found, err)
					}
					if object.GetKey() != key {
						world.ReleaseObjectState(object)
						b.Fatal("wrong object key")
					}
					if _, _, err := object.GetRootRef(ctx); err != nil {
						world.ReleaseObjectState(object)
						b.Fatal(err)
					}
					world.ReleaseObjectState(object)
				}
			}
			read()
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				read()
			}
		})
	}
}
