//go:build !js

package sobject_world_engine_test

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
)

// worldOp is one step of a test write.
type worldOp func(ctx context.Context, ws world.WorldState) error

// TestOptimisticWriteConflicts opens an optimistic write, commits another
// write while it is open, and checks that the optimistic write commits on top
// of it exactly when nothing it read changed. A changed quad touches the
// objects it names, and any change touches a read of the whole World.
func TestOptimisticWriteConflicts(t *testing.T) {
	cases := []struct {
		// name names the case.
		name string
		// read is what the optimistic write reads before the other write.
		read worldOp
		// other is what the other write does, or nil for no other write.
		other worldOp
		// conflict is whether the optimistic write must fail.
		conflict bool
	}{
		{"untouched key", getObject("occ/read"), createObject("occ/new"), false},
		{"whole World on an unchanged head", accessWorld, nil, false},
		{"read key", getObject("occ/read"), createObject("occ/read"), true},
		{"quad on a read object", getObject("occ/base"), setQuad("occ/base", "occ/linked"), true},
		{"whole World", accessWorld, createObject("occ/new"), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Open the optimistic write on a World holding two objects and
			// make its read.
			ctx := t.Context()
			w := startLocalWorld(t, t.TempDir(), nil)
			w.create(t, "occ/base")
			w.create(t, "occ/linked")
			tx := newOptimisticWrite(t, w)
			run(t, tx, tc.read)

			// Commit the other write while it is open, then its own write.
			if tc.other != nil {
				commitBeside(t, w, tc.other)
			}
			run(t, tx, createObject("occ/written"))
			err := tx.Commit(ctx)

			// It fails with a stale generation exactly on a conflict, and the
			// World holds its write exactly when it commits.
			if tc.conflict != errors.Is(err, coord.ErrStaleGeneration) || (!tc.conflict && err != nil) {
				t.Fatalf("Commit returned %v, want a conflict: %v", err, tc.conflict)
			}
			read := newRead(t, w)
			defer read.Discard()
			if worldHas(t, read, "occ/written") == tc.conflict {
				t.Fatalf("occ/written present = %v after Commit returned %v", !tc.conflict, err)
			}
		})
	}
}

// TestOptimisticWriteRerunsUnderLock runs an optimistic ExecTransaction whose
// first attempt conflicts, and checks that the rerun drops the request, sees
// the change and commits.
func TestOptimisticWriteRerunsUnderLock(t *testing.T) {
	// Run a callback that records what it found, committing a write of the
	// object it reads during the first attempt.
	w := startLocalWorld(t, t.TempDir(), nil)
	var optimistic []bool
	err := world.ExecTransaction(world.WithOptimisticWrite(t.Context()), w.eng, true, func(ctx context.Context, ws world.WorldState) error {
		// Read the object, then change it from beside the first attempt.
		optimistic = append(optimistic, world.OptimisticWrite(ctx))
		found, err := ws.HasObject(ctx, "rerun/read")
		if err != nil {
			return err
		}
		if len(optimistic) == 1 {
			commitBeside(t, w, createObject("rerun/read"))
		}

		// Write what it found.
		key := "rerun/missing"
		if found {
			key = "rerun/found"
		}
		return createObject(key)(ctx, ws)
	})
	if err != nil {
		t.Fatal(err)
	}

	// The second attempt ran under the lock and committed what it saw.
	if len(optimistic) != 2 || !optimistic[0] || optimistic[1] {
		t.Fatalf("attempts asked for optimistic writes %v, want [true false]", optimistic)
	}
	read := newRead(t, w)
	defer read.Discard()
	if !worldHas(t, read, "rerun/found") || worldHas(t, read, "rerun/missing") {
		t.Fatal("the World does not hold exactly the rerun's write")
	}
}

// TestWriteWithoutRequestHoldsLock checks that a write opened without the
// request keeps the writer lock until it commits.
func TestWriteWithoutRequestHoldsLock(t *testing.T) {
	// Open a write without the request.
	w := startLocalWorld(t, t.TempDir(), nil)
	tx := w.newWrite(t, "held/0")
	defer tx.Discard()

	// Another write cannot open while it is open.
	ctx, cancel := context.WithTimeout(t.Context(), 200*time.Millisecond)
	defer cancel()
	other, err := w.eng.NewTransaction(ctx, true)
	if err == nil {
		other.Discard()
		t.Fatal("a second write opened while a write without the request was open")
	}
	if ctx.Err() == nil || (!errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded)) {
		t.Fatalf("second write failed with %v, want the wait to end at the deadline", err)
	}

	// The first write still commits.
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// TestOptimisticWriteReceiptFailureAfterRebase fails the publication of an
// optimistic write whose reads held after another write moved the head, and
// checks that its Commit fails, the World keeps the other write without it,
// and the next write commits.
func TestOptimisticWriteReceiptFailureAfterRebase(t *testing.T) {
	// Open an optimistic write and move the head under it.
	w, so := startGatedWorld(t, t.TempDir(), nil)
	w.create(t, "rebase/0")
	tx := newOptimisticWrite(t, w)
	run(t, tx, createObject("rebase/a"))
	commitBeside(t, w, createObject("rebase/b"))

	// Fail its publication.
	injected := errors.New("injected publication failure")
	applying, release := so.hold(injected)
	done := commitAsync(t, tx)
	waitReached(t, applying)
	release()
	if err := <-done; !errors.Is(err, injected) {
		t.Fatalf("Commit: %v, want the injected failure", err)
	}

	// The World holds the other write without the failed one.
	read := newRead(t, w)
	for key, found := range map[string]bool{"rebase/0": true, "rebase/b": true, "rebase/a": false} {
		if worldHas(t, read, key) != found {
			t.Fatalf("%s present = %v after the failed Commit, want %v", key, !found, found)
		}
	}
	read.Discard()

	// The next write commits on it.
	w.create(t, "rebase/c")
	read = newRead(t, w)
	defer read.Discard()
	if !worldHas(t, read, "rebase/b") || !worldHas(t, read, "rebase/c") {
		t.Fatal("the next write does not build on the World after the failure")
	}
}

// newOptimisticWrite opens an optimistic write on w, discarded at cleanup.
func newOptimisticWrite(t *testing.T, w *localWorld) world.Tx {
	// Open the write on a context that asks to run optimistically.
	t.Helper()
	tx, err := w.eng.NewTransaction(world.WithOptimisticWrite(t.Context()), true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	return tx
}

// commitBeside commits op in a write of its own, failing when the writer lock
// is not free within a few seconds.
func commitBeside(t *testing.T, w *localWorld, op worldOp) {
	// Open the write, which waits for the writer lock.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tx, err := w.eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("open a write beside the optimistic one: %v", err)
	}
	defer tx.Discard()

	// Run op and commit.
	run(t, tx, op)
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// run runs op on tx.
func run(t *testing.T, tx world.Tx, op worldOp) {
	t.Helper()
	if err := op(t.Context(), tx); err != nil {
		t.Fatal(err)
	}
}

// getObject reads the object key.
func getObject(key string) worldOp {
	return func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := ws.GetObject(ctx, key)
		world.ReleaseObjectState(obj)
		return err
	}
}

// createObject creates the object key.
func createObject(key string) worldOp {
	return func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, key, &bucket.ObjectRef{BucketId: "local-world-bucket"})
		world.ReleaseObjectState(obj)
		return err
	}
}

// setQuad links the object subject to the object obj.
func setQuad(subject, obj string) worldOp {
	return func(ctx context.Context, ws world.WorldState) error {
		return ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(subject, "<occ-link>", obj, ""))
	}
}

// accessWorld reads the World's storage, which reads the whole World.
func accessWorld(ctx context.Context, ws world.WorldState) error {
	return ws.AccessWorldState(ctx, nil, func(*bucket_lookup.Cursor) error { return nil })
}
