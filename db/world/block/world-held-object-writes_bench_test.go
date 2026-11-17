package world_block

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"

	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// setObjectWritesHeld sets objectWritesHeld for the rest of tb.
func setObjectWritesHeld(tb testing.TB, held bool) {
	prev := objectWritesHeld
	objectWritesHeld = held
	tb.Cleanup(func() { objectWritesHeld = prev })
}

// newHeldWritesEngine opens an Engine on an okra object tree and commits
// 1,000 objects, obj/0000 through obj/0999.
func newHeldWritesEngine(tb testing.TB) *Engine {
	// Write an empty World root whose object tree is okra.
	ctx := tb.Context()
	le := logrus.NewEntry(logrus.New())
	le.Logger.SetLevel(logrus.WarnLevel)
	bed, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(bed.Release)
	base, err := bed.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(base.Release)
	btx, bcs := base.BuildTransaction(nil)
	root := NewWorld(WorldFormat_WORLD_FORMAT_CAYLEY_KV, false)
	root.ObjectKeyValue = kvtx_block.NewKeyValueStore(kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA)
	bcs.SetBlock(root, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	base.SetRootRef(rootRef)

	// Open the Engine and commit the objects.
	engine, err := NewEngine(ctx, le, base, world_mock.LookupMockOp, nil, false)
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { _ = engine.Close() })
	writer, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	for i := range 1000 {
		obj, err := writer.CreateObject(ctx, fmt.Sprintf("obj/%04d", i), nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			tb.Fatal(err)
		}
	}
	if err := writer.Commit(ctx); err != nil {
		tb.Fatal(err)
	}
	return engine
}

// heldWritesRound commits round n: it revises 200 objects rng picks, some
// twice, creates tmp/<n>/00 through tmp/<n>/19, and deletes round n-1's.
func heldWritesRound(ctx context.Context, tb testing.TB, engine *Engine, rng *rand.Rand, n int) {
	// Open the write transaction.
	writer, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		tb.Fatal(err)
	}
	defer writer.Discard()

	// Revise the picked objects.
	for range 200 {
		obj, found, err := writer.GetObject(ctx, fmt.Sprintf("obj/%04d", rng.IntN(1000)))
		if err != nil {
			tb.Fatal(err)
		}
		if !found {
			continue
		}
		if _, err := obj.IncrementRev(ctx); err != nil {
			tb.Fatal(err)
		}
		world.ReleaseObjectState(obj)
	}

	// Create this round's objects and delete the last round's.
	for i := range 20 {
		obj, err := writer.CreateObject(ctx, fmt.Sprintf("tmp/%d/%02d", n, i), nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			tb.Fatal(err)
		}
		if n > 0 {
			if _, err := writer.DeleteObject(ctx, fmt.Sprintf("tmp/%d/%02d", n-1, i)); err != nil {
				tb.Fatal(err)
			}
		}
	}
	if err := writer.Commit(ctx); err != nil {
		tb.Fatal(err)
	}
}

// TestHeldObjectWritesOffByDefault checks that a process started without
// ObjectWritesEnv applies each object write at once, as before held writes
// existed: the switch reads off, and an object write leaves nothing held in
// the write's okra object tree, where the switch turned on holds it.
func TestHeldObjectWritesOffByDefault(t *testing.T) {
	// The switch reads off unless the environment turns it on.
	if os.Getenv(ObjectWritesEnv) != "on" && objectWritesHeld {
		t.Fatalf("%s unset, yet object writes are held", ObjectWritesEnv)
	}

	// heldAfterCreate creates one object and counts the writes its tree holds.
	ctx := t.Context()
	heldAfterCreate := func(held bool) int {
		setObjectWritesHeld(t, held)
		engine := newHeldWritesEngine(t)
		writer, err := engine.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer writer.Discard()
		obj, err := writer.CreateObject(ctx, "obj/new", nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		tree, ok := writer.writeTx.state.objTree.(*kvtx_block_okra.Tx)
		if !ok {
			t.Fatalf("object tree is %T, not okra", writer.writeTx.state.objTree)
		}
		return tree.Held()
	}

	// Off applies the write at once; on holds it.
	if held := heldAfterCreate(false); held != 0 {
		t.Fatalf("default path holds %d object writes", held)
	}
	if held := heldAfterCreate(true); held == 0 {
		t.Fatal("held path holds no object writes")
	}
}

// TestHeldObjectWritesMatchImmediate checks that the held path writes what the
// immediate path writes: the same rounds of revisions, creates and deletes
// leave the same object tree root and the same objects at the same revisions.
func TestHeldObjectWritesMatchImmediate(t *testing.T) {
	// Run the same rounds with the writes immediate, then held.
	ctx := t.Context()
	type result struct {
		root *kvtx_block.KeyValueStore
		revs map[string]uint64
	}
	run := func(held bool) result {
		setObjectWritesHeld(t, held)
		engine := newHeldWritesEngine(t)
		rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a fixed test sequence.
		for n := range 4 {
			heldWritesRound(ctx, t, engine, rng, n)
		}

		// Read the committed object tree root and every object's revision.
		w, err := engine.decodeRoot(ctx, engine.GetRootRef())
		if err != nil {
			t.Fatal(err)
		}
		reader := world.NewEngineWorldState(engine, false)
		revs := make(map[string]uint64)
		iter := reader.IterateObjects(ctx, "", false)
		defer iter.Close()
		for iter.Next() {
			obj, found, err := reader.GetObject(ctx, iter.Key())
			if err != nil || !found {
				t.Fatalf("object %s: found %v, %v", iter.Key(), found, err)
			}
			_, rev, err := obj.GetRootRef(ctx)
			world.ReleaseObjectState(obj)
			if err != nil {
				t.Fatal(err)
			}
			revs[iter.Key()] = rev
		}
		if err := iter.Err(); err != nil {
			t.Fatal(err)
		}
		return result{root: w.GetObjectKeyValue(), revs: revs}
	}
	immediate, held := run(false), run(true)

	// Both paths leave the same tree and the same objects.
	if !immediate.root.EqualVT(held.root) {
		t.Fatalf("object tree roots differ:\nimmediate %v\nheld      %v", immediate.root, held.root)
	}
	if len(immediate.revs) != len(held.revs) || len(held.revs) != 1020 {
		t.Fatalf("objects: %d immediate, %d held; want 1020", len(immediate.revs), len(held.revs))
	}
	for key, rev := range immediate.revs {
		if held.revs[key] != rev {
			t.Fatalf("object %s: rev %d immediate, %d held", key, rev, held.revs[key])
		}
	}
}

// BenchmarkHeldObjectWrites measures one commit on an okra object tree that
// revises 200 of 1,000 objects, some twice, and creates and deletes 20, with
// the object tree's writes held until the commit and applied at once.
func BenchmarkHeldObjectWrites(b *testing.B) {
	for _, mode := range []struct {
		name string
		held bool
	}{{"held", true}, {"immediate", false}} {
		b.Run(mode.name, func(b *testing.B) {
			// Open the Engine, then time one round per iteration.
			setObjectWritesHeld(b, mode.held)
			engine := newHeldWritesEngine(b)
			rng := rand.New(rand.NewPCG(1, 2)) //nolint:gosec // a fixed benchmark sequence.
			b.ResetTimer()
			for n := range b.N {
				heldWritesRound(b.Context(), b, engine, rng, n)
			}
		})
	}
}
