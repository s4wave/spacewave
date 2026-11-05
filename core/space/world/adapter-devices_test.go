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
	host := so.(sobject.StateHost)
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
	ctx := t.Context()
	host := so.(sobject.StateHost)
	soID := so.GetSharedObjectID()
	err := host.GetSOHost().UpdateSOState(ctx, func(state *sobject.SOState) error {
		// Place the operation in the device's chain.
		set, err := state.OperationSet(soID)
		if err != nil {
			return err
		}
		link := state.NextOperationLink(set, d.id.String())
		if concurrent {
			link.Parents = nil
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
	})
	if err != nil {
		t.Fatal(err)
	}
}

// waitObjects waits until eng reads every key.
func waitObjects(ctx context.Context, t *testing.T, eng world.Engine, keys ...string) {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		err := world.AccessEngine(ctx, eng, func(ctx context.Context, eng world.Engine) error {
			return hasObjects(ctx, eng, keys...)
		})
		if err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal(err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// waitCheckpoint waits for a checkpoint above genesis on so.
func waitCheckpoint(ctx context.Context, t *testing.T, so sobject.SharedObject) {
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
		return err == nil && checkpoint.GetHeight() > 0, err
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
}

// TestAdapterFollowsDevices serves a Space with two other writer devices:
// the engine follows another device's commit; a device that never builds on
// the others' edits freezes the stable point, so the replay cursor bounds a
// restart; a late operation that sorts ahead of every replayed position
// replays the World from the checkpoint while local writes keep committing;
// and a remount resumes from the saved cursor with every object.
func TestAdapterFollowsDevices(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Second)
	defer cancel()
	f := newAdapterFixture(t)
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

	// A late operation sorting ahead of every position replays the World
	// while a local write commits.
	late.write(t, so, createOp(t, "late-0"), true, true)
	create("local-during")
	keys = append(keys, "late-0", "local-during")
	waitObjects(ctx, t, busEngine, keys...)

	// A remount resumes and reads every object.
	engRef.Release()
	_ = eng
	eng, engRef = f.start(t)
	defer engRef.Release()
	waitObjects(ctx, t, eng, keys...)
	if err := ctx.Err(); err != nil {
		t.Fatal(errors.Wrap(err, "run-through timed out"))
	}
}
