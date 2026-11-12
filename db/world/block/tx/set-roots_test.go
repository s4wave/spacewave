package world_block_tx_test

import (
	"context"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// rewriteRootsOp sets the root of its object twice, renames a second object
// whose root it set, and sets and deletes a third, all in one World operation.
type rewriteRootsOp struct {
	*world_mock.MockWorldOp
}

// ApplyWorldOp writes the roots.
func (o *rewriteRootsOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	key := o.GetObjectKey()
	if sys, err := world_mock.NewMockWorldOp(key, "first").ApplyWorldOp(ctx, le, ws, sender); err != nil {
		return sys, err
	}
	if sys, err := o.MockWorldOp.ApplyWorldOp(ctx, le, ws, sender); err != nil {
		return sys, err
	}
	for _, k := range []string{key + "-moved", key + "-doomed"} {
		obj, _, err := world.CreateWorldObject(ctx, ws, k, func(bcs *block.Cursor) error {
			bcs.SetBlock(&block_mock.Example{Msg: k}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return true, err
		}
	}
	obj, err := ws.RenameObject(ctx, key+"-moved", key+"-renamed", false)
	world.ReleaseObjectState(obj)
	if err != nil {
		return true, err
	}
	_, err = ws.DeleteObject(ctx, key+"-doomed")
	return true, err
}

// TestApplyWorldOpKeepsFinalRoots records one operation that sets an
// object's root twice, renames an object whose root it set and sets and
// deletes another. With the changelog off the recorded entry names each
// surviving object's final root once, in key order, and nothing for the
// deleted one. With it on, the changelog links every root the operation set,
// so the entry also names the replaced and deleted roots, in write order.
func TestApplyWorldOpKeepsFinalRoots(t *testing.T) {
	for _, tc := range []struct {
		name      string
		changelog bool
		want      []string
	}{
		{"changelog off", false, []string{"final", "obj-moved"}},
		{"changelog on", true, []string{"final", "obj-moved", "first", "obj-doomed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			le := logrus.NewEntry(logrus.New())
			tb, err := testbed.NewTestbed(ctx, le)
			if err != nil {
				t.Fatal(err)
			}
			ocs, err := tb.BuildEmptyCursor(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer ocs.Release()

			// Seed the object, commit, and record over a fork of the stored
			// World.
			ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ws.SetChangelogDisabled(ctx, !tc.changelog); err != nil {
				t.Fatal(err)
			}
			obj, err := world_block.BuildMockObject(ctx, ws, "obj")
			world.ReleaseObjectState(obj)
			if err != nil {
				t.Fatal(err)
			}
			if err := ws.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			ocs.SetRootRef(ws.GetRootRef())
			ws, err = world_block.BuildMockWorldState(ctx, le, true, ocs, false)
			if err != nil {
				t.Fatal(err)
			}
			fork, err := ws.Fork(ctx)
			if err != nil {
				t.Fatal(err)
			}
			rec, err := world_block_tx.NewWorldState(ctx, fork.(*world_block.WorldState), true)
			if err != nil {
				t.Fatal(err)
			}

			// Apply the operation and read the block at each root it names.
			if _, _, err := rec.ApplyWorldOp(ctx, &rewriteRootsOp{world_mock.NewMockWorldOp("obj", "final")}, tb.Volume.GetPeerID()); err != nil {
				t.Fatal(err)
			}
			txs := rec.GetTxBatch().GetTxs()
			roots := txs[len(txs)-1].GetTxApplyWorldOp().GetSetRoots()
			got := make([]string, len(roots))
			for i, root := range roots {
				if err := rec.AccessWorldState(ctx, &bucket.ObjectRef{RootRef: root}, func(cursor *bucket_lookup.Cursor) error {
					_, bcs := cursor.BuildTransaction(nil)
					blk, err := block.UnmarshalBlock[block_mock.Example](ctx, bcs)
					got[i] = blk.GetMsg()
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("operation names set roots holding %q, want %q", got, tc.want)
			}
		})
	}
}
