//go:build !goscript

package sobject_world_engine_test

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/world"
)

// TestWorldEngineDrainedForkReleasesBlocks checks that an abandoned
// transaction which drained part of its write buffer to the Space bucket
// leaves no block owned by the bucket alone. It writes more objects than the
// write buffer holds, so it runs only natively.
func TestWorldEngineDrainedForkReleasesBlocks(t *testing.T) {
	// Start the Space World.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	w := newSpaceWorld(ctx, t)

	// Commit one object the sweep must keep.
	err := world.ExecTransaction(ctx, w.eng, true, func(ctx context.Context, ws world.WorldState) error {
		return writeExample(ctx, ws, "kept", "kept")
	})
	if err != nil {
		t.Fatal(err)
	}

	// Abandon a transaction that wrote more blocks than its write buffer holds,
	// so part of it drained to the bucket before the abandon.
	err = world.ExecTransaction(ctx, w.eng, true, func(ctx context.Context, ws world.WorldState) error {
		for i := range 5000 {
			if err := writeExample(ctx, ws, "drained/"+strconv.Itoa(i), "drained "+strconv.Itoa(i)); err != nil {
				return err
			}
		}
		return errAbandon
	})
	if !errors.Is(err, errAbandon) {
		t.Fatal(err)
	}

	// No block may be owned by the bucket alone.
	if orphans := w.bucketOnlyBlocks(ctx, t); len(orphans) != 0 {
		t.Fatalf("%d blocks are owned only by the bucket: %v", len(orphans), orphans)
	}

	// Sweep everything unreferenced, then read back the committed object.
	if _, err := block_gc.NewCollector(w.rg, w.vol, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	err = world.ExecTransaction(ctx, w.eng, false, func(ctx context.Context, ws world.WorldState) error {
		return checkExample(ctx, ws, "kept", "kept")
	})
	if err != nil {
		t.Fatal(err)
	}
}

// TestWorldEngineDrainedForkKeepsCommittedPayloads checks that an abandoned
// transaction which drained a copy of a committed operation's payload leaves
// the payload owned, so a replay from the checkpoint still rebuilds the file.
// The payload is held only by the Space bucket, the same owner the abandoned
// transaction releases its drained blocks from.
func TestWorldEngineDrainedForkKeepsCommittedPayloads(t *testing.T) {
	// Start the Space World.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	w := newSpaceWorld(ctx, t)

	// Commit a file write whose payload the World after it does not reference.
	if err := world.ExecTransaction(ctx, w.eng, true, w.createFile); err != nil {
		t.Fatal(err)
	}
	if err := world.ExecTransaction(ctx, w.eng, true, w.appendFile); err != nil {
		t.Fatal(err)
	}

	// Abandon a transaction that writes the same payload again, then more
	// blocks than its write buffer holds, so the payload copy drains first.
	err := world.ExecTransaction(ctx, w.eng, true, func(ctx context.Context, ws world.WorldState) error {
		if err := w.appendFile(ctx, ws); err != nil {
			return err
		}
		for i := range 5000 {
			if err := writeExample(ctx, ws, "drained/"+strconv.Itoa(i), "drained "+strconv.Itoa(i)); err != nil {
				return err
			}
		}
		return errAbandon
	})
	if !errors.Is(err, errAbandon) {
		t.Fatal(err)
	}

	// Sweep everything unreferenced, then replay from the checkpoint.
	if _, err := block_gc.NewCollector(w.rg, w.vol, nil).Collect(ctx); err != nil {
		t.Fatal(err)
	}
	w.checkReplayedFile(ctx, t)
}
