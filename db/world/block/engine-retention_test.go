//go:build !js && !wasip1

package world_block

import (
	"fmt"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/world"
)

// Replacing fixed-size state must plateau after collection. Historical data
// explicitly referenced by a changelog is tested separately from this workload.
func TestEngineRetentionPlateausAndPreservesReaders(t *testing.T) {
	f := newSessionFixture(t)
	ctx := t.Context()
	store := f.engine.writeBlockStore
	collect := func() {
		t.Helper()
		if _, err := block_gc.NewCollector(f.volume.GetRefGraph(), f.volume, nil).Collect(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var firstRoot, firstBody *block.BlockRef
	var reader *Tx
	var fork *Tx
	var records, fileBytes uint64
	for i := range 64 {
		w := sessionWriter(t, f)
		if i == 0 {
			w.writeTx.state.bcs.SetBlock(NewWorld(true), true)
		}
		c, err := w.BuildStorageCursor(ctx)
		if err != nil {
			t.Fatal(err)
		}
		btx, bcs := c.BuildTransaction(nil)
		bcs.SetBlock(block_mock.NewExample(fmt.Sprintf("payload-%04d", i)), true)
		body, _, err := btx.Write(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		ref := c.GetRef().Clone()
		ref.RootRef = body
		c.Release()
		obj, found, err := w.GetObject(ctx, "object")
		if err != nil {
			t.Fatal(err)
		}
		if found {
			_, err = obj.SetRootRef(ctx, ref)
		} else {
			obj, err = w.CreateObject(ctx, "object", ref)
		}
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		if err := w.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			firstRoot, firstBody = f.engine.GetRootRef().GetRootRef(), body
			reader, err = f.engine.ForkBlockTransaction(ctx, false)
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Discard()
			fork, err = f.engine.ForkBlockTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			defer fork.Discard()
		}
		collect()
		for _, ref := range []*block.BlockRef{body, f.engine.GetRootRef().GetRootRef(), firstRoot, firstBody} {
			if i >= 16 && (ref.EqualsRef(firstRoot) || ref.EqualsRef(firstBody)) {
				continue
			}
			if found, err := store.GetBlockExists(ctx, ref); err != nil || !found {
				t.Fatalf("iteration %d lost retained block: found=%v err=%v", i, found, err)
			}
		}
		if i == 15 {
			sessionHas(t, reader, "object", true)
			reader.Discard()
			collect()
			if found, err := store.GetBlockExists(ctx, firstBody); err != nil || !found {
				t.Fatalf("fork lost its base: %v %v", found, err)
			}
			fork.Discard()
			collect()
			for _, ref := range []*block.BlockRef{firstRoot, firstBody} {
				if found, err := store.GetBlockExists(ctx, ref); err != nil || found {
					t.Fatalf("released snapshot retained: %v %v", found, err)
				}
			}
		}
		if i == 31 || i == 63 {
			stats, err := f.volume.GetStorageStats(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("writes=%d records=%d file-bytes=%d", i+1, stats.GetBlockCount(), stats.GetTotalBytes())
			if i == 31 {
				records, fileBytes = stats.GetBlockCount(), stats.GetTotalBytes()
			} else if stats.GetBlockCount() != records || stats.GetTotalBytes() > fileBytes {
				t.Fatalf("fixed live state grew: records %d -> %d, bytes %d -> %d", records, stats.GetBlockCount(), fileBytes, stats.GetTotalBytes())
			}
		}
	}
}

func TestEngineRetentionPreservesExplicitHistory(t *testing.T) {
	f := newSessionFixture(t)
	ctx := t.Context()
	w := sessionWriter(t, f)
	_, body := sessionObject(t, w, "history")
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	old := f.engine.GetRootRef().GetRootRef()
	w = sessionWriter(t, f)
	if _, err := w.DeleteObject(ctx, "history"); err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
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
