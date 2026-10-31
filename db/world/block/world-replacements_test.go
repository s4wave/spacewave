package world_block

import (
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// TestWorldReplacementsMatchDiff commits World operations that write object
// trees through the World, and checks each commit's transition against the
// diff of the old and new World roots.
func TestWorldReplacementsMatchDiff(t *testing.T) {
	// Build an empty testbed cursor.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer ocs.Release()

	// Build an engine that hands over each commit's transition.
	type transition struct {
		from, to *block.BlockRef
		step     *block.ReplacementStep
	}
	var got []transition
	eng, err := NewEngine(ctx, le, ocs, nil, nil, false, WithTransitions(func(from, to *block.BlockRef, step *block.ReplacementStep) {
		got = append(got, transition{from, to, step})
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()

	// Fork a write on it and read through the fork's buffered store.
	fork, err := eng.ForkBlockTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer fork.Discard()
	ws := fork.state
	store := ws.store

	// commit commits the pending operations and checks their transition, and
	// adds it to chain.
	from := ws.GetRootRef()
	var chain []*block.ReplacementStep
	commit := func(name string) {
		// Commit and check the one transition against the diff.
		if err := ws.Commit(ctx); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		to := ws.GetRootRef()
		if len(got) != 1 || !got[0].from.EqualsRef(from) || !got[0].to.EqualsRef(to) {
			t.Fatalf("%s: transitions %v, want one from %v to %v", name, got, from, to)
		}
		if err := block_mock.CheckReplacements(ctx, store, from, to, got[0].step.Net(nil)); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		chain = append(chain, got[0].step)
		got, from = nil, to
	}

	// writeChain writes root -> mid -> leaf through the World and returns the
	// object ref of the root.
	writeChain := func(msg string) *bucket.ObjectRef {
		var oref *bucket.ObjectRef
		err := ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
			// Build the three pages under one new root.
			oref = bls.GetRef()
			btx, root := bls.BuildTransactionAtRef(nil, nil)
			root.SetBlock(&block_mock.SubBlock{}, true)
			mid := root.FollowRef(1, nil)
			mid.SetBlock(&block_mock.SubBlock{}, true)
			mid.FollowRef(1, nil).SetBlock(block_mock.NewExample(msg), true)

			// Store them and return the root.
			var err error
			oref.RootRef, _, err = btx.Write(ctx, true)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return oref
	}

	// putRoot puts one block through a World cursor as an object root, the way
	// the Notes object block writes, and returns its object ref.
	putRoot := func(msg string) *bucket.ObjectRef {
		var oref *bucket.ObjectRef
		err := ws.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
			// Encode the block and put it as the object's root.
			data, err := block_mock.NewExample(msg).MarshalVT()
			if err != nil {
				return err
			}
			oref = bls.GetRef().Clone()
			oref.RootRef, _, err = bls.PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
		return oref
	}

	// createObject creates an object at a new chain.
	createObject := func(key string) {
		obj, err := ws.CreateObject(ctx, key, writeChain(key))
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Turning the changelog off drops entries that name Object blocks the
	// object tree also names, a second parent the record does not count, so
	// the commit forfeits its record.
	if _, err := ws.SetChangelogDisabled(ctx, true); err != nil {
		t.Fatal(err)
	}
	if err := ws.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].step != nil {
		t.Fatalf("changelog switch: transitions %v, want one forfeited", got)
	}
	got, from = nil, ws.GetRootRef()
	chainFrom := from

	// Create two objects with their trees.
	createObject("a")
	createObject("b")
	commit("create")

	// Look up a's tree.
	obj, found, err := ws.GetObject(ctx, "a")
	if err != nil || !found {
		t.Fatalf("get a: found %v: %v", found, err)
	}
	oref, _, err := obj.GetRootRef(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Edit a's leaf through an object transaction the World built.
	err = obj.AccessWorldState(ctx, oref, func(bls *bucket_lookup.Cursor) error {
		// Load the root and middle pages.
		btx, root := bls.BuildTransaction(nil)
		rootBlk, err := root.Unmarshal(ctx, block_mock.NewSubBlockBlock)
		if err != nil {
			return err
		}
		mid := root.FollowRef(1, rootBlk.(*block_mock.SubBlock).GetExamplePtr())
		if _, err := mid.Unmarshal(ctx, block_mock.NewSubBlockBlock); err != nil {
			return err
		}

		// Replace the leaf and store the new tree.
		mid.FollowRef(1, nil).SetBlock(block_mock.NewExample("a edited"), true)
		oref = oref.Clone()
		oref.RootRef, _, err = btx.Write(ctx, true)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Point a at the new tree.
	if _, err := obj.SetRootRef(ctx, oref); err != nil {
		t.Fatal(err)
	}
	world.ReleaseObjectState(obj)
	commit("edit")

	// Link a to b in the graph.
	if err := ws.SetGraphQuad(ctx, world.NewGraphQuadWithKeys("a", "<links>", "b", "")); err != nil {
		t.Fatal(err)
	}
	commit("quad")

	// Rename a to c, which moves its quad, then delete b and the quad.
	obj, err = ws.RenameObject(ctx, "a", "c", false)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	commit("rename")
	if err := ws.DeleteGraphQuad(ctx, world.NewGraphQuadWithKeys("c", "<links>", "b", "")); err != nil {
		t.Fatal(err)
	}
	if _, err := ws.DeleteObject(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	commit("delete")

	// Create an object holding one put block, then replace the block.
	obj, err = ws.CreateObject(ctx, "n", putRoot("note"))
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	commit("put")

	// Point n at a second put block.
	obj, _, err = ws.GetObject(ctx, "n")
	if err != nil {
		t.Fatal(err)
	}
	_, err = obj.SetRootRef(ctx, putRoot("note edited"))
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	commit("put edit")

	// Delete n, then store its block again under a new object: the page dies
	// in one commit and lives again in the next.
	if _, err := ws.DeleteObject(ctx, "n"); err != nil {
		t.Fatal(err)
	}
	commit("put delete")
	obj, err = ws.CreateObject(ctx, "m", putRoot("note edited"))
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	commit("put again")

	// The joined chain reports what died from the first recorded root to the
	// last.
	if err := block_mock.CheckReplacements(ctx, store, chainFrom, from, block.ReplacementSteps(chain).Join().Net(nil)); err != nil {
		t.Fatalf("chain: %v", err)
	}
}
