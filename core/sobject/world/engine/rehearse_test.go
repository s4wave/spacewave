//go:build !js

package sobject_world_engine_test

import (
	"context"
	"errors"
	"testing"

	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/world"
)

// TestRehearsalSkipsWriterLock checks that a rehearsal opens while a write
// holds the writer lock, reads the newest World, accepts writes, and leaves
// no trace: its Commit fails and neither the database nor the World sees it.
func TestRehearsalSkipsWriterLock(t *testing.T) {
	// Commit one object, then hold the writer lock with an open write.
	ctx := t.Context()
	w := startLocalWorld(t, t.TempDir(), nil)
	w.create(t, "rehearse/base")
	held := w.newWrite(t, "rehearse/held")
	before := w.seq()

	// Open a rehearsal while the write holds the lock.
	err := world.AccessEngine(ctx, w.eng, func(ctx context.Context, eng world.Engine) error {
		// Find the rehearsing engine and fork the newest World.
		re, ok := eng.(sobject_world_engine.RehearsalEngine)
		if !ok {
			t.Fatal("the World engine does not rehearse")
		}
		tx, err := re.NewRehearsalTransaction(ctx)
		if err != nil {
			return err
		}
		defer tx.Discard()

		// It reads the committed object and writes a new one.
		if !hasObject(t, ctx, tx, "rehearse/base") {
			t.Fatal("rehearsal lacks the base object")
		}
		obj, err := tx.CreateObject(ctx, "rehearse/scratch", &bucket.ObjectRef{BucketId: "local-world-bucket"})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}

		// Its Commit refuses.
		if err := tx.Commit(ctx); !errors.Is(err, sobject_world_engine.ErrRehearsalCommit) {
			t.Fatalf("rehearsal commit = %v; want ErrRehearsalCommit", err)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// The rehearsal committed nothing, and the held write still commits.
	if seq := w.seq(); seq != before {
		t.Fatalf("database sequence moved from %d to %d during the rehearsal", before, seq)
	}
	if err := held.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	read, err := w.eng.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	if hasObject(t, ctx, read, "rehearse/scratch") {
		t.Fatal("the rehearsal's object is visible")
	}
	if !hasObject(t, ctx, read, "rehearse/held") {
		t.Fatal("the held write's object is missing")
	}
}

// hasObject reports whether tx holds the object key.
func hasObject(t *testing.T, ctx context.Context, tx world.Tx, key string) bool {
	// Look the object up and release its state.
	t.Helper()
	obj, found, err := tx.GetObject(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(obj)
	return found
}
