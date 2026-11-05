package space_world_test

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// testDevice is another device's writer on a Space: a participant whose
// operations the test signs and adds to the Space's state directly, as a
// peer sync delivers them.
type testDevice struct {
	// priv signs the device's operations.
	priv crypto.PrivKey
	// id is the device's peer ID.
	id peer.ID
}

// addDevice adds a writer device to the Space hosted by so.
func addDevice(t *testing.T, so sobject.SharedObject) *testDevice {
	t.Helper()
	p, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	priv, err := p.GetPrivKey(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	host := so.(sobject.InviteHost)
	_, err = sobject.AddSOParticipant(t.Context(), host.GetSOHost(), so.GetSharedObjectID(), host.GetPrivKey(), so.GetPeerID().String(),
		p.GetPeerID().String(), p.GetPubKey(), sobject.SOParticipantRole_SOParticipantRole_WRITER, "", "")
	if err != nil {
		t.Fatal(err)
	}
	return &testDevice{priv: priv, id: p.GetPeerID()}
}

// createOp encodes the World operation that creates the object key.
func createOp(t *testing.T, key string) []byte {
	t.Helper()
	op, err := (&space_world.WorldOp{Body: &space_world.WorldOp_ApplyTxOp{ApplyTxOp: &space_world.ApplyTxOp{
		Tx: &world_block_tx.Tx{
			TxType:         world_block_tx.TxType_TxType_CREATE_OBJECT,
			TxCreateObject: &world_block_tx.TxCreateObject{ObjectKey: key},
		},
	}}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	return op
}

// write adds the device's next operation with data to the Space's state.
// concurrent leaves out the other devices' heads, so the operation follows
// only the device's own chain. first, when set, retries until the operation
// sorts first in the order, ahead of every operation above the checkpoint.
func (d *testDevice) write(t *testing.T, so sobject.SharedObject, data []byte, concurrent, first bool) {
	t.Helper()
	err := so.(sobject.InviteHost).GetSOHost().UpdateSOState(t.Context(), func(state *sobject.SOState) error {
		return d.addOp(t, so, state, data, concurrent, first, 0)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// add adds the device's next operation with data to state, as write does,
// for a caller holding the state lock.
func (d *testDevice) add(t *testing.T, so sobject.SharedObject, state *sobject.SOState, data []byte, concurrent bool) error {
	return d.addOp(t, so, state, data, concurrent, false, 0)
}

// writeAfter adds the device's next operation with data after the first n
// operations above the checkpoint only, so it acknowledges that prefix.
func (d *testDevice) writeAfter(t *testing.T, so sobject.SharedObject, data []byte, n int) {
	t.Helper()
	err := so.(sobject.InviteHost).GetSOHost().UpdateSOState(t.Context(), func(state *sobject.SOState) error {
		return d.addOp(t, so, state, data, false, false, n)
	})
	if err != nil {
		t.Fatal(err)
	}
}

// addOp signs and adds the device's next operation to state. after, when
// set, names only the operation at that position of the order as a parent.
func (d *testDevice) addOp(t *testing.T, so sobject.SharedObject, state *sobject.SOState, data []byte, concurrent, first bool, after int) error {
	// Place the operation in the device's chain.
	ctx := t.Context()
	host := so.(sobject.InviteHost)
	soID := so.GetSharedObjectID()
	set, err := state.OperationSet(soID)
	if err != nil {
		return err
	}
	link := state.NextOperationLink(set, d.id.String())
	if concurrent {
		link.Parents = nil
	}
	if after != 0 {
		link.Parents = prefixHeads(set, after)
	}

	// Encrypt it with the current key, as the device holds it.
	handle := sobject.NewSOStateParticipantHandle(nil, transform_all.BuildFactorySet(), soID, state, host.GetPrivKey(), so.GetPeerID())
	xfrm, err := handle.GetTransformer(ctx)
	if err != nil {
		return err
	}
	enc, err := xfrm.EncodeBlock(data)
	if err != nil {
		return err
	}

	// Sign it, retrying for a hash that sorts first when asked.
	for {
		op, err := sobject.BuildSOOperation(soID, d.priv, enc, link, sobject.NewSOOperationLocalID())
		if err != nil {
			return err
		}
		if first {
			trial := state.Clone()
			if _, err := trial.AddOperation(soID, op); err != nil {
				return err
			}
			order, err := trial.OperationSet(soID)
			if err != nil {
				return err
			}
			if !bytes.Equal(order.Order()[0], op.Hash()) {
				continue
			}
		}
		_, err = state.AddOperation(soID, op)
		return err
	}
}

// prefixHeads returns the operations of the first n of set's order that no
// other operation of that prefix names, so an operation naming them follows
// exactly that prefix.
func prefixHeads(set *sobject.SOOperationSet, n int) []*sobject.SOOperationPosition {
	// Mark each hash the prefix names.
	prefix := set.Order()[:n]
	named := make(map[string]struct{}, n)
	for _, h := range prefix {
		inner := set.Get(h)
		named[string(inner.GetPrevOpHash())] = struct{}{}
		for _, parent := range inner.GetParents() {
			named[string(parent.GetOpHash())] = struct{}{}
		}
	}

	// Name the rest.
	var heads []*sobject.SOOperationPosition
	for _, h := range prefix {
		if _, ok := named[string(h)]; ok {
			continue
		}
		inner := set.Get(h)
		heads = append(heads, &sobject.SOOperationPosition{PeerId: inner.GetPeerId(), Nonce: inner.GetNonce(), OpHash: h})
	}
	return heads
}

// requireComplete fails the test unless every block of the World the named
// root holds is readable from so's block store.
func requireComplete(ctx context.Context, t *testing.T, so sobject.SharedObject, name string) {
	t.Helper()
	store := so.GetBlockStore()
	root, err := store.GetRoot(ctx, name)
	if err != nil || root.GetEmpty() {
		t.Fatalf("root %s = %v, %v", name, root, err)
	}
	var holes int
	err = block.CopyGraph(ctx, store, block_mock.NewMockStore(0), root, &block.GraphCopyOptions{
		Forget: true,
		Missing: func(context.Context, *block.BlockRef) error {
			holes++
			return nil
		},
	})
	if err != nil || holes != 0 {
		t.Fatalf("World under %s has %d missing blocks: %v", name, holes, err)
	}
}

// waitObjects waits until eng reads every key, checking again after each
// World revision.
func waitObjects(ctx context.Context, t *testing.T, eng world.Engine, keys ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	ws := world.NewEngineWorldState(eng, false)
	for {
		// Read the revision, then the keys.
		seqno, err := ws.GetSeqno(ctx)
		if err == nil {
			err = world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
				return hasObjects(ctx, eng, keys...)
			})
		}
		if err == nil {
			return
		}

		// Wait for the next revision.
		if _, werr := ws.WaitSeqno(ctx, seqno+1); werr != nil {
			t.Fatal(err)
		}
	}
}

// waitCheckpoint waits for a checkpoint above genesis on so.
func waitCheckpoint(ctx context.Context, t *testing.T, so sobject.SharedObject) {
	t.Helper()
	waitCheckpointAbove(ctx, t, so, 0)
}

// checkpointHeight returns the height of so's checkpoint.
func checkpointHeight(ctx context.Context, t *testing.T, so sobject.SharedObject) uint64 {
	t.Helper()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	checkpoint, err := snap.GetCheckpoint(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return checkpoint.GetHeight()
}

// waitCheckpointAbove waits for a checkpoint above height on so.
func waitCheckpointAbove(ctx context.Context, t *testing.T, so sobject.SharedObject, height uint64) {
	t.Helper()
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	_, err = ctr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		checkpoint, err := snap.GetCheckpoint(ctx)
		return err == nil && checkpoint.GetHeight() > height, err
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// TestAdapterFollowsDevices serves a Space with two other writer devices:
// the engine follows another device's commit; a device that never builds on
// the others' edits freezes the stable point, so the replay cursor bounds a
// restart and moves several times; once both devices acknowledge a prefix
// between two cursor saves, the checkpoint of that prefix lands complete; a
// late operation that sorts ahead of every replayed position replays the
// World from the checkpoint while local writes keep committing; and a
// remount resumes from the saved cursor with every object. The frees variant
// collects between the steps on a Volume that frees at once, so a page a
// kept World names and no root holds is lost.
func TestAdapterFollowsDevices(t *testing.T) {
	t.Run("local", func(t *testing.T) { runFollowsDevices(t, newAdapterFixture(t)) })
	t.Run("frees", func(t *testing.T) {
		f := newAdapterFixtureOn(t, fixtureOpts{frees: true})
		runFollowsDevices(t, f)
		if f.freed == 0 {
			t.Fatal("the collections freed nothing, so the run-through checked no hold")
		}
	})
}

// runFollowsDevices walks TestAdapterFollowsDevices on f.
func runFollowsDevices(t *testing.T, f *adapterFixture) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	eng, engRef := f.start(t)
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	create := func(key string) {
		t.Helper()
		if err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
			obj, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}

	// The owner alone checkpoints its history.
	keys := []string{"local-0"}
	create("local-0")
	for i := range sobject.MinCheckpointOperations {
		key := "early-" + strconv.Itoa(i)
		keys = append(keys, key)
		create(key)
	}
	so := f.space(t)
	waitCheckpoint(ctx, t, so)

	// The engine follows another device's commit.
	remote := addDevice(t, so)
	remote.write(t, so, createOp(t, "remote-0"), false, false)
	waitObjects(ctx, t, busEngine, "local-0", "remote-0")

	// A third device that never writes freezes the stable point, so writes
	// past the cursor bound save the cursor and checkpoint nothing.
	late := addDevice(t, so)
	keys = append(keys, "remote-0")
	for i := range 40 {
		key := "local-" + strconv.Itoa(i+1)
		keys = append(keys, key)
		create(key)
	}
	store := so.GetBlockStore()
	waitRoot := func(name string) {
		t.Helper()
		deadline := time.Now().Add(20 * time.Second)
		for {
			ref, err := store.GetRoot(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			if !ref.GetEmpty() {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("root %s never held", name)
			}
			time.Sleep(20 * time.Millisecond)
		}
	}
	waitRoot(space_world.RootReplayCursor)
	f.collect(t)

	// More writes move the cursor again. The later ones land beside the
	// early objects, so they replace pages the first cursor World stored
	// that the World of the acknowledged prefix below still names.
	for i := range 40 {
		key := "more-" + strconv.Itoa(i)
		if i >= 9 {
			key = "early-z-" + strconv.Itoa(i)
		}
		keys = append(keys, key)
		create(key)
	}
	f.collect(t)

	// Both devices acknowledge a prefix between the cursor saves, and its
	// checkpoint lands with every object of the World it names.
	height := checkpointHeight(ctx, t, so)
	remote.writeAfter(t, so, createOp(t, "remote-ack"), 50)
	late.writeAfter(t, so, createOp(t, "late-ack"), 50)
	keys = append(keys, "remote-ack", "late-ack")
	waitCheckpointAbove(ctx, t, so, height)
	f.collect(t)
	waitObjects(ctx, t, busEngine, keys...)
	requireComplete(ctx, t, so, space_world.RootReplayBase)

	// A late operation of a device that joined after the checkpoint sorts
	// ahead of every position and replays the World while a local write
	// commits.
	later := addDevice(t, so)
	later.write(t, so, createOp(t, "late-0"), true, true)
	create("local-during")
	keys = append(keys, "late-0", "local-during")
	waitObjects(ctx, t, busEngine, keys...)

	// A remount resumes and reads every object after a collection.
	f.collect(t)
	engRef.Release()
	_ = eng
	eng, engRef = f.start(t)
	defer engRef.Release()
	waitObjects(ctx, t, eng, keys...)
	if err := ctx.Err(); err != nil {
		t.Fatal(errors.Wrap(err, "run-through timed out"))
	}
}

// TestAdapterSupersedes checks that a local write whose operation replay
// places after another device's conflicting operation, which the writer did
// not build on, is superseded and runs again on the head that holds the other
// operation.
func TestAdapterSupersedes(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	_, engRef := f.start(t)
	defer engRef.Release()
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	if err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "base", nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	so := f.space(t)
	remote := addDevice(t, so)

	// Hold the state lock while a local write builds on the head without the
	// other device's operation, then add that operation, so the local one
	// queues after it.
	host := so.(sobject.InviteHost).GetSOHost()
	var attempts int
	built := make(chan struct{})
	done := make(chan error, 1)
	err := host.UpdateSOState(ctx, func(state *sobject.SOState) error {
		// Build the local write.
		go func() {
			done <- world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
				attempts++
				if attempts == 1 {
					defer close(built)
				}
				obj, err := ws.CreateObject(ctx, "contested", nil)
				world.ReleaseObjectState(obj)
				return err
			})
		}()
		select {
		case <-built:
		case <-ctx.Done():
			return ctx.Err()
		}

		// Add the other device's conflicting operation.
		return remote.add(t, so, state, createOp(t, "contested"), false)
	})
	if err != nil {
		t.Fatal(err)
	}

	// The first attempt is superseded; the second sees the other device's
	// object.
	err = <-done
	if attempts != 2 || err == nil || errors.Is(err, sobject.ErrRejectedOp) {
		t.Fatalf("write after %d attempts returned %v; want a second attempt that finds the object", attempts, err)
	}
	waitObjects(ctx, t, busEngine, "base", "contested")
}

// TestAdapterUnmountMidCommit unmounts a Space while one commit's operation
// is inside QueueOperation and another waits in the queue behind it: the
// first reports its durability unknown, the second is superseded, neither
// hangs, and the Space mounts again.
func TestAdapterUnmountMidCommit(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	eng, engRef := f.start(t)
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	waitObjects(ctx, t, eng)
	so := f.space(t)

	// Hold the state lock, so the first operation stops inside
	// QueueOperation.
	held := make(chan struct{})
	unhold := make(chan struct{})
	go func() {
		_ = so.(sobject.InviteHost).GetSOHost().UpdateSOState(ctx, func(*sobject.SOState) error {
			close(held)
			<-unhold
			return errors.New("held only")
		})
	}()
	<-held

	// Commit twice; both admit, then wait.
	commit := func(key string) <-chan error {
		out := make(chan error, 1)
		go func() {
			tx, err := busEngine.NewTransaction(ctx, true)
			if err != nil {
				out <- err
				return
			}
			defer tx.Discard()
			obj, err := tx.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			if err == nil {
				err = tx.Commit(ctx)
			}
			out <- err
		}()
		return out
	}
	first := commit("first")
	time.Sleep(100 * time.Millisecond)
	second := commit("second")
	time.Sleep(100 * time.Millisecond)

	// Unmount: the first is unknown, the second superseded.
	engRef.Release()
	_ = eng
	for name, done := range map[string]<-chan error{"first": first, "second": second} {
		select {
		case err := <-done:
			if !errors.Is(err, world.ErrDurabilityUnknown) && !errors.Is(err, world.ErrSuperseded) {
				t.Fatalf("%s commit after unmount = %v, want unknown or superseded", name, err)
			}
		case <-ctx.Done():
			t.Fatalf("%s commit hung across the unmount", name)
		}
	}
	close(unhold)

	// The Space mounts again and serves writes.
	eng, engRef = f.start(t)
	defer engRef.Release()
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "after", nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	waitObjects(ctx, t, eng, "after")
}
