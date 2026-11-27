package space_world_test

import (
	"bytes"
	"context"
	"math"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/sobject"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/world"
	world_block_tx "github.com/s4wave/spacewave/db/world/block/tx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
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
	_, _, _, err = sobject.AddSOParticipant(t.Context(), host.GetSOHost(), so.GetSharedObjectID(), host.GetPrivKey(), so.GetPeerID().String(),
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
// set, selects only the operation at that position of the order as a parent.
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
// other operation of that prefix references, so an operation referencing them
// follows exactly that prefix.
func prefixHeads(set *sobject.SOOperationSet, n int) []*sobject.SOOperationPosition {
	// Mark each hash the prefix references.
	prefix := set.Order()[:n]
	named := make(map[string]struct{}, n)
	for _, h := range prefix {
		inner := set.Get(h)
		named[string(inner.GetPrevOpHash())] = struct{}{}
		for _, parent := range inner.GetParents() {
			named[string(parent.GetOpHash())] = struct{}{}
		}
	}

	// Record the remaining operations as heads.
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

// requireWriterAtHead fails the test unless the next local write on eng's
// Space builds on the installed head.
func requireWriterAtHead(ctx context.Context, t *testing.T, eng world.Engine) {
	t.Helper()
	var a *space_world.Adapter
	if err := world.AccessEngine(ctx, eng, func(_ context.Context, eng world.Engine) error {
		a = eng.(*space_world.Engine).GetAdapter()
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	head, err := a.Head(ctx)
	if err != nil {
		t.Fatal(err)
	}
	turn, err := a.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	base := turn.Base()
	turn.Release()
	if !base.Root.EqualsRef(head.Root) || base.Seqno != head.Seqno {
		t.Fatalf("the writer head %v is not the installed head %v", base, head)
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

// TestAdapterFollowsDevices serves a Space with two other writer devices. The
// engine follows another device's commit. A device that never builds on the
// others' edits freezes the stable point, so the replay cursor bounds a
// restart and moves several times. Once both devices acknowledge a prefix
// between two cursor saves, the checkpoint of that prefix lands complete. A
// late operation that sorts ahead of every replayed position replays the
// World from the checkpoint while local writes keep committing. A remount
// resumes from the saved cursor with every object. The frees variant collects
// between the steps on a Volume that frees at once, so a page a kept World
// references and no root holds is lost.
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

	// With nothing pending, the writer head moved with the installed head,
	// so the next local write builds on the other device's commit.
	requireWriterAtHead(ctx, t, busEngine)

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
	// that the World of the acknowledged prefix below still references.
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
	// checkpoint lands with every object of the World it references.
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
	eng, engRef = f.remount(ctx, t, eng, engRef)
	waitObjects(ctx, t, eng, keys...)

	// Writes that leave the saved cursor short of a whole save, then two
	// remounts whose installs run before replay applies anything, keep the
	// World the saved cursor ends at held through a collection, and save no
	// cursor at a restored World that left memory.
	for i := range 20 {
		key := "after-mount-" + strconv.Itoa(i)
		keys = append(keys, key)
		create(key)
	}
	for range 2 {
		eng, engRef = f.remount(ctx, t, eng, engRef)
		f.collect(t)
		waitObjects(ctx, t, eng, keys...)
	}
	defer engRef.Release()
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

	// The first attempt is superseded. The second sees the other device's
	// object.
	err = <-done
	if attempts != 2 || err == nil || errors.Is(err, sobject.ErrRejectedOp) {
		t.Fatalf("write after %d attempts returned %v; want a second attempt that finds the object", attempts, err)
	}
	waitObjects(ctx, t, busEngine, "base", "contested")
}

// TestAdapterRefusesSecondHold races a local edit that takes a hold on an
// object against another device's operation that takes it first: replay runs
// the local hold op after the other device's edge, so the local write is
// superseded and then refused, and the object keeps one holder.
func TestAdapterRefusesSecondHold(t *testing.T) {
	// Create the object and both Flowgraphs.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixtureOn(t, fixtureOpts{opTypes: &space_world.OpTypes{Lookup: optypes.LookupWorldOp}})
	_, engRef := f.start(t)
	defer engRef.Release()
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	const held, first, second = "held", "flowgraph/first", "flowgraph/second"
	if err := world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, held, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		for _, key := range []string{first, second} {
			if _, _, err := ws.ApplyWorldOp(ctx, &s4wave_flowgraph.CreateFlowgraphOp{ObjectKey: key}, ""); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	so := f.space(t)
	remote := addDevice(t, so)
	holdEdge, err := world_block_tx.NewTxSetGraphQuad(world.GraphQuadToQuad(s4wave_flowgraph.HoldQuad(second, "runtime", held)))
	if err != nil {
		t.Fatal(err)
	}
	remoteHold, err := (&space_world.WorldOp{Body: &space_world.WorldOp_ApplyTxOp{ApplyTxOp: &space_world.ApplyTxOp{Tx: holdEdge}}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}

	// Hold the state lock while the local edit builds on the head without the
	// other device's hold, then add that hold, so the local edit queues after
	// it.
	host := so.(sobject.InviteHost).GetSOHost()
	var attempts int
	built := make(chan struct{})
	done := make(chan error, 1)
	err = host.UpdateSOState(ctx, func(state *sobject.SOState) error {
		// Build the local edit.
		go func() {
			done <- world.ExecTransaction(ctx, busEngine, true, func(ctx context.Context, ws world.WorldState) error {
				attempts++
				if attempts == 1 {
					defer close(built)
				}
				_, err := s4wave_flowgraph.UpdateFlowgraph(ctx, ws, first, &s4wave_flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*s4wave_flowgraph.FlowgraphNode{
					"runtime": {TypeId: "fixture", ObjectKey: held, Requirements: &s4wave_flowgraph.FlowgraphRequirements{}},
				}})
				return err
			})
		}()
		select {
		case <-built:
		case <-ctx.Done():
			return ctx.Err()
		}

		// Add the other device's hold.
		return remote.add(t, so, state, remoteHold, false)
	})
	if err != nil {
		t.Fatal(err)
	}

	// The first attempt is superseded. The second finds the other hold.
	err = <-done
	if attempts != 2 || err == nil || !strings.Contains(err.Error(), "is held by") {
		t.Fatalf("edit after %d attempts returned %v; want a second attempt refused by the other hold", attempts, err)
	}
	if err := world.ExecTransaction(ctx, busEngine, false, func(ctx context.Context, ws world.WorldState) error {
		edges, err := ws.LookupGraphQuads(ctx, world.NewGraphQuadWithKeys("", s4wave_flowgraph.HoldPredicate, held, ""), 0)
		if err == nil && (len(edges) != 1 || edges[0].GetSubject() != world.KeyToGraphValue(second).String()) {
			err = errors.Errorf("hold edges = %v, want the other device's alone", edges)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// TestAdapterAdmitsOnMovedHead begins a write on the head H, lets replay
// apply another device's conflicting operation, which moves the head to H',
// and only then commits it: the write is admitted on H' and applies there,
// so it is refused or superseded, and never replaces the other device's
// object.
func TestAdapterAdmitsOnMovedHead(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	_, engRef := f.start(t)
	defer engRef.Release()
	busEngine := world.NewBusEngine(ctx, f.tb.Bus, f.engineID)
	waitObjects(ctx, t, busEngine)
	so := f.space(t)
	remote := addDevice(t, so)

	// Begin the write on H.
	tx, err := busEngine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, err := tx.CreateObject(ctx, "contested", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}

	// Move the head to H' with the other device's operation.
	remote.write(t, so, createOp(t, "contested"), false, false)
	waitObjects(ctx, t, busEngine, "contested")

	// Commit: the write is admitted on H' and does not replace the object.
	err = tx.Commit(ctx)
	if err == nil || (!world.IsSuperseded(err) && !errors.Is(err, sobject.ErrRejectedOp)) {
		t.Fatalf("commit on the moved head = %v, want superseded or refused", err)
	}
	waitObjects(ctx, t, busEngine, "contested")
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

	// Commit twice. Both writes are admitted, then wait.
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

// TestWriteWaitsThroughOpening holds an order of heldOps operations from
// another device while the Space World is down, mounts it again, and writes at
// once: the write waits while replay opens, is admitted once replay is caught
// up, and lands after every held operation. It logs how long the write waited.
func TestWriteWaitsThroughOpening(t *testing.T) {
	const heldOps = 256
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	eng, engRef := f.start(t)
	waitObjects(ctx, t, eng)
	so := f.space(t)
	remote := addDevice(t, so)

	// Unmount, then hold the order the next mount replays.
	keys := make([]string, heldOps)
	for i := range keys {
		keys[i] = "held-" + strconv.Itoa(i)
	}
	var served *space_world.Engine
	if err := world.AccessEngine(ctx, eng, func(_ context.Context, eng world.Engine) error {
		served = eng.(*space_world.Engine)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	engRef.Release()
	if _, err := served.WaitSeqno(ctx, math.MaxUint64); !world.IsSuperseded(err) {
		t.Fatalf("waiting for the released engine to retire: %v", err)
	}
	if err := so.(sobject.InviteHost).GetSOHost().UpdateSOState(ctx, func(state *sobject.SOState) error {
		for _, key := range keys {
			if err := remote.add(t, so, state, createOp(t, key), false); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	// Mount and write at once: the write waits for replay to open, and
	// commits behind every held operation.
	begin := time.Now()
	eng, engRef = f.start(t)
	defer engRef.Release()
	mounted := time.Since(begin)
	if err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		obj, err := ws.CreateObject(ctx, "local", nil)
		world.ReleaseObjectState(obj)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	t.Logf("a write sent at mount committed %v after the mount began (mount returned at %v) behind %d held operations", time.Since(begin), mounted, heldOps)
	if err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
		return hasObjects(ctx, eng, append(keys, "local")...)
	}); err != nil {
		t.Fatalf("the write committed ahead of the held order: %v", err)
	}
}

// TestInspectOrderReportsUndecodableOps has another device order a body this
// build does not declare, data that does not decode, a known body beside an
// unknown WorldOp field, the removed body field 3 and a transaction, and
// checks that InspectOrder reports each operation with what this build reads
// of it and the outcome the saved cursor records there, counts the order, and
// writes nothing.
func TestInspectOrderReportsUndecodableOps(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	_, engRef := f.start(t)
	so := f.space(t)
	health := spaceHealth(ctx, t, so)
	waitHealth(ctx, t, health, "caught up", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplay().GetCaughtUp() != nil
	})

	// Stop the engine, which the inspection does not need, so nothing else
	// writes. Another device then orders a newer build's body, invalid data,
	// a known body beside a newer field, a removed body and a create.
	engRef.Release()
	remote := addDevice(t, so)
	remote.write(t, so, unreadableOp(), false, false)
	remote.write(t, so, []byte{0xff}, false, false)
	remote.write(t, so, hiddenFieldOp(t, "hidden-object"), false, false)
	remote.write(t, so, removedOp(), false, false)
	remote.write(t, so, createOp(t, "after-object"), false, false)

	// Wait for the state to order the five operations, then save a cursor
	// that refused the newer body as an unknown operation type.
	ctr, rel, err := so.AccessSharedObjectState(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer rel()
	var ev *sobject.OrderEvent
	_, err = ctr.WaitValueWithValidator(ctx, func(snap sobject.SharedObjectStateSnapshot) (bool, error) {
		if snap == nil {
			return false, nil
		}
		var err error
		ev, err = sobject.ReadOrder(ctx, snap)
		return err == nil && ev.Set.CountAbove(remote.id.String(), 0) == 5, err
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cursor := &space_world.ReplayCursor{Height: ev.Checkpoint.GetHeight()}
	for _, h := range ev.Order {
		reason := ""
		if inner := ev.Set.Get(h); inner.GetPeerId() == remote.id.String() && inner.GetNonce() == 1 {
			reason = "unknown operation type"
		}
		cursor.Outcomes = append(cursor.Outcomes, &space_world.ReplayCursorOutcome{Hash: h, Reason: reason})
	}
	if err := space_world.WriteSavedCursor(ctx, so, cursor); err != nil {
		t.Fatal(err)
	}

	// Inspect the order.
	var unreadable, invalid, removed []*space_world.OrderOperation
	var seen int
	summary, err := space_world.InspectOrder(ctx, so, func(op *space_world.OrderOperation) error {
		if int(op.GetPosition()) != seen {
			t.Fatalf("operation at position %d, want %d", op.GetPosition(), seen)
		}
		seen++
		switch {
		case op.GetUnreadable() != nil:
			unreadable = append(unreadable, op)
		case op.GetInvalid() != nil:
			invalid = append(invalid, op)
		case op.GetRefused().GetReason() == "removed operation":
			removed = append(removed, op)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Each undecodable operation is the other device's, and only the newer
	// body carries the saved refusal. The known body beside a newer field
	// is unreadable as replay reads it, reporting only the unknown field.
	if len(unreadable) != 2 || !slices.Equal(unreadable[0].GetUnreadable().GetFields(), []uint32{99}) || !slices.Equal(unreadable[1].GetUnreadable().GetFields(), []uint32{9}) {
		t.Fatalf("unreadable operations = %v, want field 99, then field 9 beside a known body", unreadable)
	}
	if op := unreadable[0]; op.GetPeerId() != remote.id.String() || !op.SavedUnknownType() || !op.Undecodable() {
		t.Fatalf("unreadable operation = %v, want the other device's, saved as an unknown operation type", op)
	}
	if op := unreadable[1]; op.GetPeerId() != remote.id.String() || op.GetNonce() != 3 || op.SavedUnknownType() || !op.Undecodable() {
		t.Fatalf("hidden-field operation = %v, want the other device's third, with no saved refusal", op)
	}
	if len(removed) != 1 || removed[0].GetNonce() != 4 || removed[0].Undecodable() {
		t.Fatalf("removed operations = %v, want the other device's fourth, refused as removed", removed)
	}
	if len(invalid) != 1 || invalid[0].GetSavedReason() != "" || !invalid[0].Undecodable() {
		t.Fatalf("invalid operations = %v, want one with no saved refusal", invalid)
	}

	// The summary counts the whole order and the saved refusal.
	var applyTx uint32
	for _, k := range summary.GetKinds() {
		if k.GetKind() == space_world.WorldOpKind_WORLD_OP_KIND_APPLY_TX {
			applyTx = k.GetCount()
		}
	}
	if int(summary.GetOperations()) != len(ev.Order) || seen != len(ev.Order) {
		t.Fatalf("summary counts %d operations and the walk %d, want %d", summary.GetOperations(), seen, len(ev.Order))
	}
	if summary.GetUnreadable() != 2 || summary.GetInvalid() != 1 || summary.GetRefused() == 0 || applyTx == 0 {
		t.Fatalf("summary = %v, want two unreadable, one invalid, the removed refusal and the create", summary)
	}
	if summary.GetCursor().GetUnknownOperationType() != 1 {
		t.Fatalf("saved cursor summary = %v, want one unknown operation type", summary.GetCursor())
	}

	// The inspection left the saved cursor as it was.
	after, err := space_world.ReadSavedCursor(ctx, so)
	if err != nil {
		t.Fatal(err)
	}
	if !after.EqualVT(cursor) {
		t.Fatal("the inspection changed the saved cursor")
	}
}

// unreadableOp encodes a World operation a newer build wrote: a WorldOp whose
// only field is a body this build does not declare, field 99.
func unreadableOp() []byte {
	body := "a body only a newer build reads"
	return append([]byte{0x9a, 0x06, byte(len(body))}, body...)
}

// hiddenFieldOp encodes a World operation a newer build wrote beside a body
// this build reads: a transaction that creates key, with WorldOp field 9,
// which this build does not declare.
func hiddenFieldOp(t *testing.T, key string) []byte {
	field := "a field only a newer build reads"
	return append(createOp(t, key), append([]byte{0x4a, byte(len(field))}, field...)...)
}

// removedOp encodes a World operation with the removed body field 3, which
// releases v0.57.20 through v0.57.22 wrote.
func removedOp() []byte {
	return []byte{0x1a, 0x00}
}

// TestReplayWaitsAtUnreadableOp tries to make a replica apply an operation past
// one whose body its build cannot read. Another device orders an operation with
// an unknown body, then a transaction that creates hidden-object beside an
// unknown WorldOp field, then one that creates after-object. Replay waits for a
// build that reads the first, with the World unchanged, and refuses a write. A
// restart waits there again rather than adopt an outcome. The owner's skip of
// it, which every replica reads, moves the wait to the second, whose body
// replay must not apply without the field. Skipping that too applies
// after-object, and only then can ImportRoots reference the payloads. An
// operation with the removed body field 3 is refused, not waited at.
func TestReplayWaitsAtUnreadableOp(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
	eng, engRef := f.start(t)
	defer func() { engRef.Release() }()
	so := f.space(t)
	health := spaceHealth(ctx, t, so)
	create := func(key string) error {
		return world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
			obj, err := ws.CreateObject(ctx, key, nil)
			world.ReleaseObjectState(obj)
			return err
		})
	}
	if err := create("before-object"); err != nil {
		t.Fatal(err)
	}
	waitHealth(ctx, t, health, "caught up", func(h *sobject.SharedObjectHealth) bool {
		return h.GetReplay().GetCaughtUp() != nil
	})
	seqno, err := eng.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Another device orders the unreadable operations, then a later one.
	remote := addDevice(t, so)
	remote.write(t, so, unreadableOp(), false, false)
	remote.write(t, so, hiddenFieldOp(t, "hidden-object"), false, false)
	remote.write(t, so, createOp(t, "after-object"), false, false)
	unread := waitUpgrade(ctx, t, so, health, remote)

	// The World is the one before the operations, and a write is refused.
	checkBefore := func() {
		t.Helper()
		if got, err := eng.GetSeqno(ctx); err != nil || got != seqno {
			t.Fatalf("seqno while replay waits = %d, %v, want %d", got, err, seqno)
		}
		if err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
			if err := hasObjects(ctx, eng, "before-object"); err != nil {
				return err
			}
			if err := lacksObject(ctx, eng, "hidden-object"); err != nil {
				return err
			}
			return lacksObject(ctx, eng, "after-object")
		}); err != nil {
			t.Fatal(err)
		}
	}
	importRoots := func() error {
		t.Helper()
		snap, err := so.GetSharedObjectState(ctx)
		if err != nil {
			t.Fatal(err)
		}
		_, err = space_world.ImportRoots(ctx, f.tb.Logger, f.tb.Bus, so, f.engineID, space_world.OpTypes{}, snap)
		return err
	}
	checkBefore()
	if err := create("waiting-write"); !errors.Is(err, sobject.ErrReplayWaits) {
		t.Fatalf("write while replay waits at an unreadable body = %v, want the typed wait", err)
	}
	if err := importRoots(); !errors.Is(err, space_world.ErrUnreadableOp) {
		t.Fatalf("import roots at an unreadable body = %v, want ErrUnreadableOp", err)
	}

	// A restart on the same build waits at the operation again.
	eng, engRef = f.remount(ctx, t, eng, engRef)
	health = spaceHealth(ctx, t, so)
	if again := waitUpgrade(ctx, t, so, health, remote); !again.EqualVT(unread) {
		t.Fatalf("wait after a restart at %v, want %v", again, unread)
	}
	checkBefore()

	// The owner skips the operation: replay waits at the known body beside
	// an unknown field instead of applying it.
	host := so.(sobject.InviteHost)
	skip := func(pos *sobject.SOOperationPosition) {
		t.Helper()
		if err := sobject.SkipSOOperation(ctx, host.GetSOHost(), pos, "a newer build wrote it", host.GetPrivKey()); err != nil {
			t.Fatal(err)
		}
	}
	skip(unread)
	first := health.GetValue().GetReplay().GetWaiting().GetPosition()
	var past *sobject.SharedObjectHealth
	waitHealth(ctx, t, health, "the wait past the skip", func(h *sobject.SharedObjectHealth) bool {
		past = h
		wait := h.GetReplay().GetWaiting()
		return wait.GetUpgrade() != nil && wait.GetPosition() != first || h.GetReplay().GetCaughtUp() != nil
	})
	if past.GetReplay().GetCaughtUp() != nil {
		t.Fatal("replay applied a body beside a WorldOp field this build does not declare")
	}
	hidden := orderPosition(ctx, t, so, past.GetReplay().GetWaiting().GetPosition())
	if hidden.GetPeerId() != remote.id.String() || hidden.GetNonce() != unread.GetNonce()+1 {
		t.Fatalf("replay waits at %v, want the other device's next operation after %v", hidden, unread)
	}
	checkBefore()
	if err := importRoots(); !errors.Is(err, space_world.ErrUnreadableOp) {
		t.Fatalf("import roots at a hidden field = %v, want ErrUnreadableOp", err)
	}

	// Skipping it too applies the later transaction, and ImportRoots
	// follows both skips.
	skip(hidden)
	waitObjects(ctx, t, eng, "before-object", "after-object")
	if err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
		return lacksObject(ctx, eng, "hidden-object")
	}); err != nil {
		t.Fatal(err)
	}
	if err := importRoots(); err != nil {
		t.Fatalf("import roots after the skips = %v", err)
	}

	// Replay refuses the removed body and applies the transaction after it.
	remote.write(t, so, removedOp(), false, false)
	remote.write(t, so, createOp(t, "final-object"), false, false)
	waitObjects(ctx, t, eng, "final-object")
}

// orderPosition returns the position of the operation at index i of the
// agreed order of so.
func orderPosition(ctx context.Context, t *testing.T, so sobject.SharedObject, i uint32) *sobject.SOOperationPosition {
	t.Helper()
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order := set.Order()
	if int(i) >= len(order) {
		t.Fatalf("position %d of an order of %d", i, len(order))
	}
	inner := set.Get(order[i])
	return &sobject.SOOperationPosition{PeerId: inner.GetPeerId(), Nonce: inner.GetNonce(), OpHash: order[i]}
}

// waitUpgrade waits for replay of so to wait for a build that reads the
// operation of remote it stopped at, and returns that operation's position.
func waitUpgrade(ctx context.Context, t *testing.T, so sobject.SharedObject, health ccontainer.Watchable[*sobject.SharedObjectHealth], remote *testDevice) *sobject.SOOperationPosition {
	t.Helper()
	var wait *sobject.SOReplayWait
	waitHealth(ctx, t, health, "the upgrade wait", func(h *sobject.SharedObjectHealth) bool {
		wait = h.GetReplay().GetWaiting()
		return wait.GetUpgrade() != nil
	})
	snap, err := so.GetSharedObjectState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	set, err := snap.GetOperationSet(ctx)
	if err != nil {
		t.Fatal(err)
	}
	order := set.Order()
	if int(wait.GetPosition()) >= len(order) {
		t.Fatalf("wait at position %d of an order of %d", wait.GetPosition(), len(order))
	}
	h := order[wait.GetPosition()]
	inner := set.Get(h)
	for _, prev := range order[:wait.GetPosition()] {
		if set.Get(prev).GetPeerId() == remote.id.String() {
			t.Fatal("replay waits past the other device's first operation")
		}
	}
	if inner.GetPeerId() != remote.id.String() {
		t.Fatalf("replay waits at an operation of %s, want the other device's", inner.GetPeerId())
	}
	return &sobject.SOOperationPosition{PeerId: inner.GetPeerId(), Nonce: inner.GetNonce(), OpHash: h}
}
