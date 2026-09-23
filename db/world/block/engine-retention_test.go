//go:build !js && !wasip1

package world_block

import (
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
)

// TestEngineRetentionPlateausAndPreservesReaders checks that replacing fixed-size
// state bounds retained records and payload bytes while preserving live readers.
func TestEngineRetentionPlateausAndPreservesReaders(t *testing.T) {
	// Use the durable volume with explicit collection after each World commit.
	f := newSessionFixture(t)
	ctx := t.Context()
	store := f.engine.writeBlockStore
	kvStore := f.volume.(volume_kvtx.KvtxVolume).GetKvtxStore()
	collect := func() {
		t.Helper()
		if _, err := block_gc.NewCollector(f.volume.GetRefGraph(), f.volume, nil).Collect(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Retain the first snapshot until both its reader and writable fork close.
	var firstRoot, firstBody *block.BlockRef
	var reader *Tx
	var fork *Tx
	var records, liveBytes uint64
	for i := range 64 {
		// Disable explicit history for the fixed-state workload.
		w := sessionWriter(t, f)
		if i == 0 {
			w.writeTx.state.bcs.SetBlock(NewWorld(true), true)
		}

		// Replace the object with a distinct, fixed-size body.
		c, err := w.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		btx, bcs := c.BuildTransaction(nil)
		bcs.SetBlock(block_mock.NewExample("payload-"+strconv.Itoa(1000+i)), true)
		body, _, err := btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		ref := c.GetRef().Clone()
		ref.RootRef = body
		c.Release()

		// Publish the body through the same World object on every commit.
		obj, found, err := w.GetObject(ctx, "object")
		if err != nil {
			t.Fatal(err)
		}
		switch {
		case found:
			_, err = obj.SetRootRef(ctx, ref)
		default:
			obj, err = w.CreateObject(ctx, "object", ref)
		}
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}

		// Pin the first committed snapshot with independent reader lifetimes.
		if i == 0 {
			firstRoot, firstBody = f.engine.GetRootRef().GetRootRef(), body
			reader, err = f.engine.ForkBlockTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(reader.Discard)
			fork, err = f.engine.ForkBlockTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(fork.Discard)
		}

		// Collection must preserve the current World and every pinned snapshot.
		collect()
		for _, ref := range []*block.BlockRef{body, f.engine.GetRootRef().GetRootRef(), firstRoot, firstBody} {
			if i >= 16 && (ref.EqualsRef(firstRoot) || ref.EqualsRef(firstBody)) {
				continue
			}
			if found, err := store.GetBlockExists(ctx, ref); err != nil || !found {
				t.Fatalf("iteration %d lost retained block: found=%v err=%v", i, found, err)
			}
		}

		// Closing one reader must preserve the fork's snapshot until it closes.
		if i == 15 {
			sessionHas(t, reader, "object", true)
			reader.Discard()
			collect()
			if found, err := store.GetBlockExists(ctx, firstBody); err != nil || !found {
				t.Fatalf("fork lost its base: %v %v", found, err)
			}

			// Closing the final fork makes its obsolete snapshot collectible.
			fork.Discard()
			collect()
			for _, ref := range []*block.BlockRef{firstRoot, firstBody} {
				if found, err := store.GetBlockExists(ctx, ref); err != nil || found {
					t.Fatalf("released snapshot retained: %v %v", found, err)
				}
			}
		}

		// Compare retained data after warmup and another complete write window.
		if i == 31 || i == 63 {
			// Keep physical file size as diagnostic evidence of allocator growth.
			stats, err := f.volume.GetStorageStats(ctx)
			if err != nil {
				t.Fatal(err)
			}

			// Count every retained key and value, including volume metadata.
			// File size minus free pages includes unused allocation beyond bbolt's
			// high-water mark and cannot measure live data.
			readTx, err := kvStore.NewTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			var currentLiveBytes uint64
			err = readTx.ScanPrefix(ctx, nil, func(key, value []byte) error {
				currentLiveBytes += uint64(len(key) + len(value))
				return nil
			})
			readTx.Discard()
			if err != nil {
				t.Fatal(err)
			}

			// Fixed-size replacement must not accumulate records or payload bytes.
			t.Logf(
				"writes=%d records=%d live-bytes=%d file-bytes=%d",
				i+1,
				stats.GetBlockCount(),
				currentLiveBytes,
				stats.GetTotalBytes(),
			)
			if i == 31 {
				records, liveBytes = stats.GetBlockCount(), currentLiveBytes
				continue
			}
			if stats.GetBlockCount() != records {
				t.Fatalf("fixed live state changed record count: %d -> %d", records, stats.GetBlockCount())
			}
			if currentLiveBytes > liveBytes {
				t.Fatalf("fixed live state grew: bytes %d -> %d", liveBytes, currentLiveBytes)
			}
		}
	}
}

// TestEngineRetentionPreservesExplicitHistory checks that collection retains
// historical bodies referenced by the current World's changelog.
func TestEngineRetentionPreservesExplicitHistory(t *testing.T) {
	// Commit an object with the default history retention enabled.
	f := newSessionFixture(t)
	ctx := t.Context()
	w := sessionWriter(t, f)
	_, body := sessionObject(t, w, "history")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	old := f.engine.GetRootRef().GetRootRef()

	// Delete the object while preserving its explicit historical reference.
	w = sessionWriter(t, f)
	if _, err := w.DeleteObject(ctx, "history"); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Collection releases the superseded World but keeps its historical body.
	if _, err := block_gc.NewCollector(f.volume.GetRefGraph(), f.volume, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	if found, err := f.volume.GetBlockExists(ctx, old); err != nil || found {
		t.Fatalf("superseded World retained: %v %v", found, err)
	}
	if found, err := f.volume.GetBlockExists(ctx, body.GetRootRef()); err != nil || !found {
		t.Fatalf("explicit historical body lost: %v %v", found, err)
	}
}
