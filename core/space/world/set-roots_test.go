package space_world_test

import (
	"bytes"
	"context"
	"crypto/rand"
	"slices"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
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
	"github.com/s4wave/spacewave/db/volume/device"
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

// requireKeypair reads the keypair at key through eng and fails unless it has
// peer ID peerID.
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

// requireDurableSetRoots fails unless every root that a stored World operation
// above the checkpoint holds is in the Space's durable block store, which a
// restart reads without the adapter's kept pages, resolving operation types
// with lookup. It returns how many set roots the operations recorded.
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

// TestAdmitHoldsEarlyDrains commits one transaction too large for the write
// buffer, whose capacity drains reach only the writer's stage and never the
// candidate's entries, on a Volume that frees every block no root holds once
// the grace passes. After the transaction ended, it collects, reads every
// object at the head, writes a checkpoint over it, and reads them again after
// a remount loads that checkpoint.
func TestAdmitHoldsEarlyDrains(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	eng, engRef := f.start(t)
	defer func() { engRef.Release() }()

	// Note whether a kept candidate reports blocks drained to its stage.
	var staged atomic.Bool
	defer space_world.SetAdmittedHook(func(c world.Candidate) {
		if c.Staged {
			staged.Store(true)
		}
	})()

	// Create more objects in one transaction than the buffer's 4,096 entries.
	keys := make([]string, 6000)
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		for i := range keys {
			keys[i] = "drain-" + strconv.Itoa(i)
			obj, err := ws.CreateObject(ctx, keys[i], nil)
			world.ReleaseObjectState(obj)
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if !staged.Load() {
		t.Fatal("no candidate reported a drain to its stage: the test no longer covers one")
	}

	// readAll fails unless the head serves every object promptly: a page the
	// collector freed would leave a read waiting.
	readAll := func(when string) {
		t.Helper()
		readCtx, readCancel := context.WithTimeout(ctx, 20*time.Second)
		defer readCancel()
		if err := hasObjects(readCtx, eng, keys...); err != nil {
			t.Fatalf("%s: %v", when, err)
		}
	}

	// Collect past the grace with the writer's stage released, then read.
	f.collect(t)
	readAll("after a collection")

	// Write a checkpoint over the transaction, collect, and read it back from
	// a remount.
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
	f.collect(t)
	eng, engRef = f.remount(ctx, t, eng, engRef)
	readAll("after a checkpoint and a remount")
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

// TestSetRootNeverDurableUnheld tries to find a durable operation that
// references a root the store may drop: after a write abandoned between its
// hold and its operation's state write, and after a power cut at each point of
// a write on a store that orders the hold before that state write.
func TestSetRootNeverDurableUnheld(t *testing.T) {
	t.Run("abandoned", testSetRootAbandoned)
	t.Run("power cut", testSetRootPowerCut)
}

// testSetRootAbandoned tries to find a durable operation that
// references a root the store may drop. After each write, and with no
// checkpoint, it reads every stored operation above the checkpoint, the way a
// restart or an account replica does, and requires each root the operation's
// payload refs list to be in the durable store: a KeypairUpdateOp's staged
// root, and an object root whose graph nests chunk blocks below it, which must
// read back whole after a remount. The holds commit with write ordering only,
// as on a store whose state writes order them. A write abandoned after its
// hold and before its operation's state write, as a crash between them leaves
// it, must leave no operation behind, and after a remount and a collection
// that frees every block no root holds, no stored operation may name a block
// the collection freed.
func testSetRootAbandoned(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	defer space_world.ForceOrderedHolds()()
	f := newAdapterFixtureOn(t, fixtureOpts{frees: true, opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
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

	// Abandon a write with nested blocks after its ordered hold, before its
	// operation's state write.
	abandon := errors.New("abandoned after the hold")
	var ordered atomic.Bool
	clearHeld := space_world.SetHeldHook(func(o bool) error {
		ordered.Store(o)
		return abandon
	})
	lost := make([]byte, len(data))
	if _, err := rand.Read(lost); err != nil {
		t.Fatal(err)
	}
	err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, ws, "abandoned", func(bcs *block.Cursor) error {
			_, err := blob.BuildBlobWithBytes(ctx, lost, bcs)
			return err
		})
		world.ReleaseObjectState(obj)
		return err
	})
	clearHeld()
	if err == nil {
		t.Fatal("abandoned write committed")
	}
	if !ordered.Load() {
		t.Fatal("abandoned write's hold was not ordered")
	}

	// Remount and read both through replay.
	eng, engRef = f.remount(ctx, t, eng, engRef)
	defer engRef.Release()
	requireKeypair(ctx, t, "after remount", eng, key, peerID)

	// Free every block no root holds, after another write lets the adapter
	// drop what it held for the abandoned one: every stored operation's roots
	// must stay, and the abandoned write must have left no operation.
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "after-abandon", nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.collect(t)
	requireDurableSetRoots(ctx, t, so, optypes.LookupWorldOp)
	if err := world.ExecTransaction(ctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		obj, found, err := ws.GetObject(ctx, "abandoned")
		world.ReleaseObjectState(obj)
		if err == nil && found {
			return errors.New("abandoned write applied")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
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

// testSetRootPowerCut cuts power under writes on the s4db store the desktop
// runs, which orders each admitted write's hold before its operation's state
// write: write n loses power at its n-th device call, counting the deadline
// flush that may follow it, and the device keeps what a power loss could
// leave. Since the hold commits ordered, every call comes after it. After each
// cut it boots again and, before the World engine starts and replay holds
// anything, frees every block no durable hold keeps. Every stored operation's
// roots must then be stored, and every blob that reads back must read whole.
// A blob that read back after an earlier cut, or whose write returned
// without an error, must read back after every later one. A write returns
// after its cut when its state commit failed: the store may still show the
// commit, so the write must report its outcome unknown rather than stored.
// The cuts end at the first write that makes no call at its cut point. At least
// one cut must lose a write after its hold, or the run did not test the
// window the ordering closes.
func testSetRootPowerCut(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Minute)
	defer cancel()
	lookup := optypes.LookupWorldOp
	dev := device.NewMemory()
	f := newAdapterFixtureOn(t, fixtureOpts{dev: dev, frees: true, opTypes: &space_world.OpTypes{Lookup: lookup}})
	eng, engRef := f.start(t)
	defer func() { engRef.Release() }()
	key, peerID := storeTestKeypair(ctx, t, eng)

	// Note each hold an admitted write takes, and any that was not ordered.
	var held, unordered atomic.Bool
	defer space_world.SetHeldHook(func(ordered bool) error {
		held.Store(true)
		unordered.Store(unordered.Load() || !ordered)
		return nil
	})()

	// write stores a blob with nested chunks under k.
	blobs := make(map[string][]byte)
	write := func(k string) error {
		data := make([]byte, 1<<20)
		if _, err := rand.Read(data); err != nil {
			t.Fatal(err)
		}
		blobs[k] = data
		wctx, wcancel := context.WithTimeout(ctx, 20*time.Second)
		defer wcancel()
		return writeBlob(wctx, eng, k, data)
	}

	// Write once uncut, which must hold ordered on s4db.
	if err := write("uncut"); err != nil {
		t.Fatal(err)
	}
	if unordered.Load() || !held.Load() {
		t.Fatal("the write on s4db took no ordered hold")
	}

	// check boots after a power cut, frees what no durable hold keeps, and
	// reads every blob back. found holds each blob that must read back.
	found := map[string]bool{"uncut": true}
	check := func(phase string, seed uint64) {
		t.Helper()
		engRef.Release()
		f.powerCut(t, seed)
		f.collect(t)
		requireDurableSetRoots(ctx, t, f.space(t), lookup)
		eng, engRef = f.start(t)
		requireKeypair(ctx, t, phase, eng, key, peerID)
		for k, data := range blobs {
			got, ok, err := readBlob(ctx, eng, k)
			if err != nil {
				t.Fatalf("%s: blob %s: %v; %s", phase, k, err, f.collectReport(ctx))
			}
			if !ok {
				if found[k] {
					t.Fatalf("%s: durable blob %s is gone", phase, k)
				}
				continue
			}
			found[k] = true
			if !bytes.Equal(got, data) {
				t.Fatalf("%s: blob %s read %d bytes, want %d", phase, k, len(got), len(data))
			}
		}
	}

	// Cut power at each device call of a write and the flush that follows
	// it. A write that returned before its cut may still wait in the queue
	// at the next boot, so the final boot checks it.
	var heldCut []string
	acked := []string{"uncut"}
	cuts := 0
	for n := range 64 {
		k := "cut-" + strconv.Itoa(n)
		held.Store(false)
		dev.CrashAfter(n)
		if err := write(k); err == nil {
			acked = append(acked, k)
		}

		// Wait out the ordered flush deadline for the cut.
		select {
		case <-f.bootDev.cut:
		case <-time.After(3 * time.Second):
		}
		if !f.bootDev.isCut() {
			dev.CrashAfter(-1)
			break
		}
		cuts++
		if held.Load() {
			heldCut = append(heldCut, k)
		}
		check("after cut "+strconv.Itoa(n), uint64(n))
	}
	check("after a clean boot", uint64(cuts))
	for _, k := range acked {
		if !found[k] {
			t.Fatalf("write %s returned without an error but is gone", k)
		}
	}
	if unordered.Load() {
		t.Fatal("a write on s4db took a hold that was not ordered")
	}

	// Require a cut between a hold and its operation's durable state write.
	window := 0
	for _, k := range heldCut {
		if !found[k] {
			window++
		}
	}
	if window == 0 {
		t.Fatalf("no cut of %d landed between a hold and its operation's state write", cuts)
	}
	t.Logf("%d cuts, %d after the hold and before the state write was durable", cuts, window)
}

// writeBlob stores data as a blob in a new object at k.
func writeBlob(ctx context.Context, eng world.Engine, k string, data []byte) error {
	return world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, _, err := world.CreateWorldObject(ctx, ws, k, func(bcs *block.Cursor) error {
			_, err := blob.BuildBlobWithBytes(ctx, data, bcs)
			return err
		})
		world.ReleaseObjectState(obj)
		return err
	})
}

// readBlob reads the blob of the object at k, reporting whether it exists.
// A block the store lost fails the read once its timeout passes.
func readBlob(ctx context.Context, eng world.Engine, k string) ([]byte, bool, error) {
	rctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	var got []byte
	var found bool
	err := world.ExecTransaction(rctx, eng, false, func(ctx context.Context, ws world.WorldState) error {
		obj, ok, err := ws.GetObject(ctx, k)
		defer world.ReleaseObjectState(obj)
		if err != nil || !ok {
			return err
		}
		found = true
		_, _, err = world.AccessObjectState(ctx, obj, false, func(bcs *block.Cursor) error {
			got, err = blob.FetchToBytes(ctx, bcs)
			return err
		})
		return err
	})
	return got, found, err
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
// operation records only the first object's final root. With it on, the
// committed changelog links every root the operation set, so the operation
// records each of them, final roots first. Every root it records is durable
// once the write returns and reads back after a remount that replays it.
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

			// Read the roots the stored operation records.
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

			// Remount before any stored World covers the operation
			// and read every recorded root back.
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
// updates it. The stored batch references each recorded root, so each must be
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
// file re-chunks the written blob into the file. The stored operation
// references the blob's root, so it must be durable.
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
