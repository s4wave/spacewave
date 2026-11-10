package world_block_test

import (
	"testing"

	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/sirupsen/logrus"
)

// TestObjectModSeqno walks creates, links, renames, deletes and a discarded
// write through an Engine on each object tree, and checks that every record a
// commit wrote carries that commit's seqno, no other record changes, and no
// deleted key comes back.
func TestObjectModSeqno(t *testing.T) {
	for _, impl := range []kvtx_block.KVImplType{
		kvtx_block.KVImplType_KV_IMPL_TYPE_IAVL,
		kvtx_block.KVImplType_KV_IMPL_TYPE_OKRA,
	} {
		t.Run(impl.String(), func(t *testing.T) {
			testObjectModSeqno(t, impl)
		})
	}
}

func testObjectModSeqno(t *testing.T, impl kvtx_block.KVImplType) {
	// Write an empty World root whose object tree uses impl.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(ocs.Release)

	// Write the root block and point the cursor at it.
	btx, bcs := ocs.BuildTransaction(nil)
	root := world_block.NewWorld(false)
	root.ObjectKeyValue = kvtx_block.NewKeyValueStore(impl)
	bcs.SetBlock(root, true)
	rootRef, _, err := btx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	ocs.SetRootRef(rootRef)

	// Open the Engine on it.
	engine, err := world_block.NewEngine(ctx, le, ocs, world_mock.LookupMockOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = engine.Close() })
	reader := world.NewEngineWorldState(engine, false)

	// commit runs fn in a write transaction and commits it.
	commit := func(fn func(tx *world_block.EngineTx)) {
		// Open a write transaction, run fn, and commit.
		t.Helper()
		tx, err := engine.NewBlockEngineTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Discard()
		fn(tx)
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// create adds objects with no root ref.
	create := func(tx *world_block.EngineTx, keys ...string) {
		t.Helper()
		for _, key := range keys {
			obj, err := tx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				t.Fatal(err)
			}
		}
	}

	// want maps each key to its expected ModSeqno and rev; zero means absent.
	type entry struct{ mod, rev uint64 }
	want := make(map[string]entry)
	keys := []string{"a", "b", "c", "d", "e"}
	check := func(step string) uint64 {
		// Read every key in one page with its seqno.
		t.Helper()
		bodies, _, seqno, err := world.GetObjectBodiesBatchPageWithSeqno(ctx, reader, keys, 1<<20)
		if err != nil {
			t.Fatal(err)
		}
		for _, body := range bodies {
			w := want[body.ObjectKey]
			if body.Exists != (w.rev != 0) {
				t.Fatalf("%s: %s exists=%v, want %v", step, body.ObjectKey, body.Exists, w.rev != 0)
			}
			if body.ModSeqno > seqno {
				t.Fatalf("%s: %s mod seqno %d after page seqno %d", step, body.ObjectKey, body.ModSeqno, seqno)
			}
			if body.ModSeqno != w.mod || body.Rev != w.rev {
				t.Fatalf("%s: %s mod=%d rev=%d, want mod=%d rev=%d", step, body.ObjectKey, body.ModSeqno, body.Rev, w.mod, w.rev)
			}
		}
		return seqno
	}

	// seqno reads the World seqno.
	seqno := func() uint64 {
		t.Helper()
		s, err := reader.GetSeqno(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	// Two creates in one commit carry its seqno, which the transaction
	// reports before it commits: the flush writes both into one entry.
	var pending uint64
	commit(func(tx *world_block.EngineTx) {
		create(tx, "a", "b")
		var err error
		if pending, err = tx.GetSeqno(ctx); err != nil {
			t.Fatal(err)
		}
	})
	s1 := seqno()
	if pending != s1 {
		t.Fatalf("transaction reported seqno %d, commit reached %d", pending, s1)
	}
	want["a"], want["b"] = entry{s1, 1}, entry{s1, 1}
	check("create")

	// A link stamps only the created object: a relationship leaves the
	// records it names alone.
	commit(func(tx *world_block.EngineTx) {
		create(tx, "c")
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("a", "<rel>", "c", "")); err != nil {
			t.Fatal(err)
		}
	})
	s2 := seqno()
	want["c"] = entry{s2, 1}
	check("link")

	// A rename stamps the new key; neither the old key nor an object created
	// and deleted in the same commit comes back.
	commit(func(tx *world_block.EngineTx) {
		// Rename c, then create and delete e.
		obj, err := tx.RenameObject(ctx, "c", "d", false)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
		create(tx, "e")
		if _, err := tx.DeleteObject(ctx, "e"); err != nil {
			t.Fatal(err)
		}
	})
	s3 := seqno()
	bodies, _, _, err := world.GetObjectBodiesBatchPageWithSeqno(ctx, reader, []string{"d"}, 1<<20)
	if err != nil || len(bodies) != 1 {
		t.Fatalf("read d: %v", err)
	}
	delete(want, "c")
	want["d"] = entry{s3, bodies[0].Rev}
	if check("rename") != s3 {
		t.Fatal("page seqno differs from the World seqno")
	}

	// A discarded write stamps nothing.
	tx, err := engine.NewBlockEngineTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}

	// Bump b's revision, then discard the transaction.
	obj, err := world.MustGetObject(ctx, tx, "b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := obj.IncrementRev(ctx); err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(obj)
	tx.Discard()

	// The seqno and every record stay as they were.
	if seqno() != s3 {
		t.Fatal("discarded write moved the seqno")
	}
	check("discard")

	// Deleting a removes its link and leaves the other end's record alone.
	commit(func(tx *world_block.EngineTx) {
		if _, err := tx.DeleteObject(ctx, "a"); err != nil {
			t.Fatal(err)
		}
	})
	if seqno() == s3 {
		t.Fatal("deleting a did not commit")
	}
	delete(want, "a")
	check("delete")

	// Reading through a write transaction reports the same records.
	commit(func(tx *world_block.EngineTx) {
		// Read b's ModSeqno through the transaction.
		obj, err := world.MustGetObject(ctx, tx, "b")
		if err != nil {
			t.Fatal(err)
		}
		defer world.ReleaseObjectState(obj)
		mod, err := world.GetObjectModSeqno(ctx, obj)
		if err != nil || mod != s1 {
			t.Fatalf("b mod seqno %d err %v, want %d", mod, err, s1)
		}
	})
}
