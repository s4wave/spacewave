//go:build !js && !wasip1

package world_block

import (
	"strconv"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// stageBody writes a body block through the stage and returns its object ref.
func stageBody(t *testing.T, stage world.WorldStage, msg string) *bucket.ObjectRef {
	// Build a storage cursor whose writes the stage owns.
	t.Helper()
	ctx := t.Context()
	c, err := stage.BuildStorageCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Release()

	// Write the body and point the cursor's ref at it.
	btx, bcs := c.BuildTransaction(nil)
	bcs.SetBlock(block_mock.NewExample(msg), true)
	body, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	ref := c.GetRef().Clone()
	ref.RootRef = body
	return ref
}

// TestStageWorldStateReclaimsUnadoptedBuild checks that a stage keeps its
// builds until release, after which only the build a commit adopted remains.
func TestStageWorldStateReclaimsUnadoptedBuild(t *testing.T) {
	// Stage two builds on the coordinated engine.
	f := newSessionFixture(t)
	ctx := t.Context()
	store := f.engine.writeBlockStore
	stage, err := f.engine.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	kept := stageBody(t, stage, "kept")
	dropped := stageBody(t, stage, "dropped")
	collect := func(want map[*block.BlockRef]bool) {
		t.Helper()
		if _, err := block_gc.NewCollector(f.volume.GetRefGraph(), f.volume, nil).Collect(ctx); err != nil {
			t.Fatal(err)
		}
		for ref, exists := range want {
			if found, err := store.GetBlockExists(ctx, ref); err != nil || found != exists {
				t.Fatalf("block %s want=%v found=%v err=%v", ref.MarshalString(), exists, found, err)
			}
		}
	}

	// The open stage keeps both builds through a sweep.
	collect(map[*block.BlockRef]bool{kept.GetRootRef(): true, dropped.GetRootRef(): true})

	// Adopt one build, then release the stage.
	w := sessionWriter(t, f)
	obj, err := w.CreateObject(ctx, "object", kept)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	stage.Release()

	// A later commit must not publish the unadopted build either.
	w = sessionWriter(t, f)
	other, err := w.CreateObject(ctx, "other", nil)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(other)
	if err := w.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	collect(map[*block.BlockRef]bool{kept.GetRootRef(): true, dropped.GetRootRef(): false})

	// The released stage rejects further writes.
	c, err := stage.BuildStorageCursor(ctx)
	if err == nil {
		c.Release()
		t.Fatal("released stage built a cursor")
	}
}

// TestAccessObjectStateReclaimsReplayedBuild checks that a publishing object
// access stages its builds, so a build replayed after a conflict and a write
// the published root never references are both reclaimed.
func TestAccessObjectStateReclaimsReplayedBuild(t *testing.T) {
	// Create the object on the engine World state.
	f := newSessionFixture(t)
	ctx := t.Context()
	ws := world.NewEngineWorldState(f.engine, true)
	obj, err := ws.CreateObject(ctx, "object", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer world.ReleaseObjectState(obj)

	// Write a stray block on each attempt and conflict with the first.
	var strays []*block.BlockRef
	outRef, _, err := world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
		// Write this attempt's stray block.
		store, _ := bcs.GetBlockStore()
		stray, _, err := store.PutBlock(ctx, []byte("stray "+strconv.Itoa(len(strays))), nil)
		if err != nil {
			return err
		}
		strays = append(strays, stray)

		// Move the revision on the first attempt to force a replay.
		if len(strays) == 1 {
			if _, err := obj.IncrementRev(ctx); err != nil {
				return err
			}
		}

		// Publish a new object root.
		bcs.SetBlock(block_mock.NewExample("published"), true)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(strays) != 2 {
		t.Fatalf("expected one replay, got %d attempts", len(strays))
	}

	// Only the published root survives a sweep.
	if _, err := block_gc.NewCollector(f.volume.GetRefGraph(), f.volume, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	want := map[*block.BlockRef]bool{outRef.GetRootRef(): true, strays[0]: false, strays[1]: false}
	for ref, exists := range want {
		if found, err := f.engine.writeBlockStore.GetBlockExists(ctx, ref); err != nil || found != exists {
			t.Fatalf("block %s want=%v found=%v err=%v", ref.MarshalString(), exists, found, err)
		}
	}
}
