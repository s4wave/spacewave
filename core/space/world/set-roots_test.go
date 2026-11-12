package space_world_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	world_mock "github.com/s4wave/spacewave/db/world/mock"
	"github.com/s4wave/spacewave/identity"
	identity_world "github.com/s4wave/spacewave/identity/world"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// storeTestKeypair stores a new keypair through KeypairUpdateOp, whose apply
// sets the object root to a block it staged outside any payload, and returns
// the keypair's object key and peer ID.
func storeTestKeypair(ctx context.Context, t *testing.T, eng world.Engine) (string, string) {
	t.Helper()
	_, _, pub, err := peer.NewPeerWithGenerateED25519()
	if err != nil {
		t.Fatal(err)
	}
	kp, err := identity.NewKeypair(pub, "", nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		_, _, err := identity_world.StoreKeypair(ctx, ws, "", kp, false)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return identity_world.NewKeypairKey(kp.GetPeerId()), kp.GetPeerId()
}

// requireKeypair reads the keypair at key through eng and fails unless it
// names peerID.
func requireKeypair(ctx context.Context, t *testing.T, phase string, eng world.Engine, key, peerID string) {
	t.Helper()
	rctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	var got string
	err := world.ExecTransaction(rctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		body, err := identity_world.LookupKeypairBody(ctx, ws, key)
		got = body.GetPeerId()
		return err
	})
	if err != nil || got != peerID {
		t.Fatalf("%s: keypair %s read peer %q, want %q: %v", phase, key, got, peerID, err)
	}
}

// storedTxs returns the recorded transaction of every stored World operation
// above the checkpoint, read the way a restart or an account replica reads
// them, without applying them.
func storedTxs(ctx context.Context, t *testing.T, so sobject.SharedObject) []*world_block_tx.Tx {
	t.Helper()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var txs []*world_block_tx.Tx
	for _, h := range set.Order() {
		inner := set.Get(h)
		if inner.IsAcknowledgment() {
			continue
		}
		_, data, reason, err := sobject.PrepareReplayOp(ctx, snap, inner)
		if err != nil || reason != "" {
			continue
		}
		op := &space_world.WorldOp{}
		if op.UnmarshalVT(data) != nil || op.GetApplyTxOp().GetTx() == nil {
			continue
		}
		txs = append(txs, op.GetApplyTxOp().GetTx())
	}
	return txs
}

// requireDurableSetRoots fails unless every root that a stored World
// operation above the checkpoint holds is in the Space's durable block store,
// which a restart reads without the adapter's kept pages, resolving operation
// types with lookup. It returns how many set roots the operations named.
func requireDurableSetRoots(ctx context.Context, t *testing.T, so sobject.SharedObject, lookup world.LookupOp) int {
	t.Helper()
	store := so.GetBlockStore()
	named := 0
	for i, tx := range storedTxs(ctx, t, so) {
		named += len(tx.GetTxApplyWorldOp().GetSetRoots()) + len(tx.GetTxApplyObjectOp().GetSetRoots())
		refs, err := tx.PayloadRefs(ctx, lookup)
		if err != nil {
			t.Fatalf("operation %d: payload refs: %v", i, err)
		}
		for _, ref := range refs {
			exists, err := store.GetBlockExists(ctx, ref)
			if err != nil || !exists {
				t.Fatalf("operation %d is durable but its root %s is not stored: exists=%v err=%v",
					i, ref.MarshalString(), exists, err)
			}
		}
	}
	return named
}

// TestSetRootSurvivesCheckpointAndRemount stores a keypair, then reads it
// after the write, after a checkpoint install sweeps the adapter's pages, and
// after a remount replays.
func TestSetRootSurvivesCheckpointAndRemount(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 90*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	key, peerID := storeTestKeypair(ctx, t, eng)
	requireKeypair(ctx, t, "after write", eng, key, peerID)

	// Commit enough operations for a checkpoint, whose install sweeps.
	for i := range sobject.MinCheckpointOperations + 1 {
		k := "fill-" + strconv.Itoa(i)
		if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			obj, err := ws.CreateObject(ctx, k, nil)
			world.ReleaseObjectState(obj)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	waitCheckpoint(ctx, t, f.space(t))
	requireKeypair(ctx, t, "after checkpoint", eng, key, peerID)

	eng, engRef = f.remount(ctx, t, eng, engRef)
	defer engRef.Release()
	requireKeypair(ctx, t, "after remount", eng, key, peerID)
}

// TestSetRootReplaysAboveStoredWorld remounts before any stored World covers
// a KeypairUpdateOp, so the restart drops every kept page and replay applies
// the operation again: the block it set must be durable.
func TestSetRootReplaysAboveStoredWorld(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	key, peerID := storeTestKeypair(ctx, t, eng)
	if h := checkpointHeight(ctx, t, f.space(t)); h != 0 {
		t.Fatalf("checkpoint at %d covers the operation; the test needs it above", h)
	}

	eng, engRef = f.remount(ctx, t, eng, engRef)
	defer engRef.Release()
	requireKeypair(ctx, t, "after remount", eng, key, peerID)
}

// TestSetRootNeverDurableUnheld tries to find a durable operation that names
// a root the store may drop. After each write, and with no checkpoint, it
// reads every stored operation above the checkpoint, the way a restart or an
// account replica does, and requires each root the operation's payload refs
// name to be in the durable store: a KeypairUpdateOp's staged root, and an
// object root whose graph nests chunk blocks below it, which must read back
// whole after a remount.
func TestSetRootNeverDurableUnheld(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	so := f.space(t)

	// The staged root of a World operation.
	key, peerID := storeTestKeypair(ctx, t, eng)
	if named := requireDurableSetRoots(ctx, t, so, optypes.LookupWorldOp); named == 0 {
		t.Fatalf("no stored operation names a set root; is %s=off?", "SPACEWAVE_WORLD_SET_ROOT_HOLDS")
	}

	// A root whose graph nests blocks below it.
	data := make([]byte, 3<<20)
	if _, err := rand.Read(data); err != nil {
		t.Fatal(err)
	}
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, ws, "nested", func(bcs *block.Cursor) error {
			_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
			return err
		})
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	requireDurableSetRoots(ctx, t, so, optypes.LookupWorldOp)
	if h := checkpointHeight(ctx, t, so); h != 0 {
		t.Fatalf("checkpoint at %d covers the operations; the test needs them above", h)
	}

	// Remount and read both through replay.
	eng, engRef = f.remount(ctx, t, eng, engRef)
	defer engRef.Release()
	requireKeypair(ctx, t, "after remount", eng, key, peerID)
	var got []byte
	if err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := ws.GetObject(ctx, "nested")
		defer world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
			got, err = blob.FetchToBytes(ctx, bcs)
			return err
		})
		return err
	}); err != nil {
		t.Fatalf("nested blob after remount: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("nested blob after remount read %d bytes, want %d", len(got), len(data))
	}
}

// rewriteOpTypeID is the type ID of rewriteOp.
const rewriteOpTypeID = "test/space-world/rewrite"

// rewriteOp sets the root of its object twice, then creates an object with a
// root and deletes it, all in one World operation.
type rewriteOp struct {
	*world_mock.MockWorldOp
}

// TypeID returns the operation type identifier.
func (o *rewriteOp) TypeID() string {
	return rewriteOpTypeID
}

// ApplyWorldOp writes the object's root twice and a doomed object's once.
func (o *rewriteOp) ApplyWorldOp(ctx context.Context, le *logrus.Entry, ws world.WorldState, sender peer.ID) (bool, error) {
	if sys, err := world_mock.NewMockWorldOp(o.GetObjectKey(), o.GetNextMsg()+" first").ApplyWorldOp(ctx, le, ws, sender); err != nil {
		return sys, err
	}
	if sys, err := o.MockWorldOp.ApplyWorldOp(ctx, le, ws, sender); err != nil {
		return sys, err
	}
	doomed := o.GetObjectKey() + "-doomed"
	obj, _, err := world.CreateWorldObject(ctx, ws, doomed, func(bcs *block.Cursor) error {
		bcs.SetBlock(&block_mock.Example{Msg: "doomed"}, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return true, err
	}
	_, err = ws.DeleteObject(ctx, doomed)
	return true, err
}

// lookupRewriteOp resolves rewriteOp.
func lookupRewriteOp(_ context.Context, opTypeID string) (world.Operation, error) {
	if opTypeID != rewriteOpTypeID {
		return nil, nil
	}
	return &rewriteOp{MockWorldOp: &world_mock.MockWorldOp{}}, nil
}

// rewriteMsgs reads the mock block at each root through ws, using the
// transform of the object at key, and returns the messages in order.
func rewriteMsgs(ctx context.Context, ws world.WorldState, key string, roots []*block.BlockRef) ([]string, error) {
	obj, err := world.MustGetObject(ctx, ws, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	objRef, _, err := obj.GetRootRef(ctx)
	if err != nil {
		return nil, err
	}
	msgs := make([]string, len(roots))
	for i, root := range roots {
		ref := objRef.CloneVT()
		ref.RootRef = root
		if err := ws.AccessWorldState(ctx, ref, func(cursor *bucket_lookup.Cursor) error {
			_, bcs := cursor.BuildTransaction(nil)
			blk, err := block.UnmarshalBlock[block_mock.Example](ctx, bcs)
			msgs[i] = blk.GetMsg()
			return err
		}); err != nil {
			return nil, err
		}
	}
	return msgs, nil
}

// TestSetRootKeepsFinalRoot applies one operation that sets an object's root
// twice and sets and deletes another's. With the changelog off the stored
// operation names only the first object's final root. With it on, the
// committed changelog links every root the operation set, so the operation
// names each of them, final roots first. Every root it names is durable once
// the write returns and reads back after a remount that replays it.
func TestSetRootKeepsFinalRoot(t *testing.T) {
	for _, tc := range []struct {
		name      string
		changelog bool
		want      []string
	}{
		{"changelog off", false, []string{"final"}},
		{"changelog on", true, []string{"final", "final first", "doomed"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
			defer cancel()
			lookup := world.LookupOpSlice{optypes.LookupWorldOp, lookupRewriteOp}.LookupOp
			f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{Lookup: lookup}})
			eng, engRef := f.start(t)
			so := f.space(t)

			// Choose the changelog and create the object.
			if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
				settings := &space_world.SpaceSettings{ChangelogEnabled: tc.changelog}
				if _, _, err := space_world_ops.SetSpaceSettings(ctx, ws, "", "", settings, true, time.Now()); err != nil {
					return err
				}
				obj, err := ws.CreateObject(ctx, "rewritten", nil)
				world.ReleaseObjectState(obj)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
				_, _, err := ws.ApplyWorldOp(ctx, &rewriteOp{MockWorldOp: world_mock.NewMockWorldOp("rewritten", "final")}, "")
				return err
			}); err != nil {
				t.Fatal(err)
			}

			// Read the roots the stored operation names.
			requireDurableSetRoots(ctx, t, so, lookup)
			var roots []*block.BlockRef
			for _, tx := range storedTxs(ctx, t, so) {
				if tx.GetTxApplyWorldOp().GetOperationTypeId() == rewriteOpTypeID {
					roots = tx.GetTxApplyWorldOp().GetSetRoots()
				}
			}
			if h := checkpointHeight(ctx, t, so); h != 0 {
				t.Fatalf("checkpoint at %d covers the operation; the test needs it above", h)
			}

			// Remount before any stored World covers the operation and read
			// every named root back.
			eng, engRef = f.remount(ctx, t, eng, engRef)
			defer engRef.Release()
			var got []string
			if err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
				var err error
				got, err = rewriteMsgs(ctx, ws, "rewritten", roots)
				return err
			}); err != nil {
				t.Fatalf("set roots after remount: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("operation names set roots holding %q, want %q", got, tc.want)
			}
		})
	}
}

// TestSetRootReplacedInTx records roots that one transaction sets and then
// replaces or deletes, as a GLaDOS drain does when it creates a light and then
// updates it. The stored batch names each recorded root, so each must be
// durable even though the committed World reaches none of them.
func TestSetRootReplacedInTx(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	defer engRef.Release()
	so := f.space(t)

	// Create an object, then replace its root.
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, ws, "light", func(bcs *block.Cursor) error {
			bcs.SetBlock(&block_mock.Example{Msg: "green"}, true)
			return nil
		})
		defer world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		_, _, err = world.AccessObjectState(ctx, obj, true, func(bcs *block.Cursor) error {
			bcs.ClearAllRefs()
			bcs.SetBlock(&block_mock.Example{Msg: "yellow"}, true)
			return nil
		})
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Create an object, then delete it.
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, ws, "scratch", func(bcs *block.Cursor) error {
			bcs.SetBlock(&block_mock.Example{Msg: "scratch"}, true)
			return nil
		})
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		_, err = ws.DeleteObject(ctx, "scratch")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	requireDurableSetRoots(ctx, t, so, optypes.LookupWorldOp)
}

// TestPayloadBuiltInTx applies a payload operation whose payload the
// transaction builds and the committed World does not reach: an append to a
// file re-chunks the written blob into the file. The stored operation names
// the blob's root, so it must be durable.
func TestPayloadBuiltInTx(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	defer engRef.Release()
	so := f.space(t)

	// Create the filesystem and a file in it.
	ts := time.Now()
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		if _, _, err := unixfs_world.FsInit(ctx, ws, "", "fs", unixfs_world.FSType_FSType_FS_NODE, nil, false, ts); err != nil {
			return err
		}
		obj, _, err := ws.GetObject(ctx, "fs")
		defer world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		_, _, err = unixfs_world.FsMknod(ctx, obj, "", unixfs_world.FSType_FSType_FS_NODE, [][]string{{"f"}}, unixfs.NewFSCursorNodeType_File(), 0o644, ts)
		return err
	}); err != nil {
		t.Fatal(err)
	}

	// Append to the file, building the payload blob in the transaction.
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := ws.GetObject(ctx, "fs")
		defer world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		_, _, err = unixfs_world.FsWriteAt(ctx, ws, obj, "", unixfs_world.FSType_FSType_FS_NODE, []string{"f"}, 0, []byte("appended"), ts)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	requireDurableSetRoots(ctx, t, so, optypes.LookupWorldOp)
}
