package s4wave_device_world_test

import (
	"context"
	"errors"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	"github.com/s4wave/spacewave/db/world/testbed"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
	s4wave_device_world "github.com/s4wave/spacewave/sdk/device/world"
)

// TestEnsureEnrolledDevice checks ready projection, revision idempotence,
// preservation of saved capability state, and rejection of conflicting identity.
func TestEnsureEnrolledDevice(t *testing.T) {
	// Project a real peer into a fresh World using the public enrollment API.
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	local, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := local.GetPeerID()
	key, err := s4wave_device_world.EnsureEnrolledDevice(ctx, tb.Engine, id, "Desktop")
	if err != nil {
		t.Fatal(err)
	}
	tx, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tx.Discard)
	if err := world_types.CheckObjectType(ctx, tx, key, s4wave_device.DeviceTypeID); err != nil {
		t.Fatal(err)
	}
	device, _, err := world.LookupObject[*s4wave_device.Device](ctx, tx, key, s4wave_device.NewDeviceBlock)
	if err != nil {
		t.Fatal(err)
	}
	if device.GetPeerId() != id.String() || !device.IsSelectable() {
		t.Fatal("mounted peer did not become a selectable Device", device)
	}

	// Retain metadata written by Device management while completing setup again.
	device.Label = "Saved device name"
	device.SetupState = s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_COMPLETION_IMPORTED
	device.UpdateState = s4wave_device.DeviceUpdateState_DEVICE_UPDATE_STATE_READY
	device.Capabilities = []*s4wave_device.DeviceCapability{{Id: "terminal", Kind: "terminal"}}
	if _, _, err := world.AccessWorldObject(ctx, tx, key, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(device, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx.Discard()
	if _, err := s4wave_device_world.EnsureEnrolledDevice(ctx, tb.Engine, id, "New label"); err != nil {
		t.Fatal(err)
	}
	read, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Discard)
	updated, _, err := world.LookupObject[*s4wave_device.Device](ctx, read, key, s4wave_device.NewDeviceBlock)
	read.Discard()
	if err != nil {
		t.Fatal(err)
	}
	if !updated.IsSelectable() || updated.Label != device.Label || updated.UpdateState != device.UpdateState || !updated.CreatedAt.EqualVT(device.CreatedAt) || len(updated.Capabilities) != 1 || !updated.Capabilities[0].EqualVT(device.Capabilities[0]) {
		t.Fatal("setup projection replaced existing Device metadata", updated)
	}

	// Repeated projection is a no-op; missing mount identity cannot write state.
	before, err := tb.Engine.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if again, err := s4wave_device_world.EnsureEnrolledDevice(ctx, tb.Engine, id, "Desktop"); err != nil || again != key {
		t.Fatalf("ready projection changed Device: %q, %v", again, err)
	}
	if _, err := s4wave_device_world.EnsureEnrolledDevice(ctx, tb.Engine, "", "Desktop"); err == nil {
		t.Fatal("missing authenticated peer accepted")
	}
	after, err := tb.Engine.GetSeqno(ctx)
	if err != nil || before != after {
		t.Fatalf("no-op projection changed revision: %d -> %d, %v", before, after, err)
	}

	// A conflicting peer at the canonical key must not be overwritten.
	write, err := tb.Engine.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(write.Discard)
	updated.PeerId = "another-peer"
	if _, _, err := world.AccessWorldObject(ctx, write, key, true, func(cursor *block.Cursor) error {
		cursor.SetBlock(updated, true)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := write.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	write.Discard()
	if _, err := s4wave_device_world.EnsureEnrolledDevice(ctx, tb.Engine, id, "Desktop"); err == nil {
		t.Fatal("conflicting Device peer overwritten")
	}
}

// TestEnsureEnrolledDeviceAcceptedAfterRejectedCreate recovers enrollment when
// a later Device create is rejected after the accepted World already has the
// matching ready Device.
func TestEnsureEnrolledDeviceAcceptedAfterRejectedCreate(t *testing.T) {
	ctx := t.Context()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	local, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	id := local.GetPeerID()
	rejected := errors.New("rejected op: transaction apply failed: tx_batch[0]: object already exists")
	engine := &applyThenRejectEngine{Engine: tb.Engine, err: rejected}

	// The write applies, then Commit reports the live duplicate-create rejection.
	key, err := s4wave_device_world.EnsureEnrolledDevice(ctx, engine, id, "Desktop")
	if err != nil {
		t.Fatal(err)
	}

	// Accepted state is the enrollment result; a missing Device keeps the write error.
	read, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(read.Discard)
	device, _, err := world.LookupObject[*s4wave_device.Device](ctx, read, key, s4wave_device.NewDeviceBlock)
	read.Discard()
	if err != nil {
		t.Fatal(err)
	}
	if device.GetPeerId() != id.String() || !device.IsSelectable() {
		t.Fatal("rejected create did not retain the accepted Device", device)
	}
	other, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err)
	}
	missing := &rejectWithoutApplyEngine{Engine: tb.Engine, err: rejected}
	if _, err := s4wave_device_world.EnsureEnrolledDevice(ctx, missing, other.GetPeerID(), "Other desktop"); !errors.Is(err, rejected) {
		t.Fatalf("missing Device error = %v, want rejected create", err)
	}
}

type applyThenRejectEngine struct {
	world.Engine
	err error
}

type applyThenRejectTx struct {
	world.Tx
	err error
}

func (e *applyThenRejectEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &applyThenRejectTx{Tx: tx, err: e.err}, nil
}

func (t *applyThenRejectTx) Commit(ctx context.Context) error {
	if err := t.Tx.Commit(ctx); err != nil {
		return err
	}
	return t.err
}

type rejectWithoutApplyEngine struct {
	world.Engine
	err error
}

type rejectWithoutApplyTx struct {
	world.Tx
	err error
}

func (e *rejectWithoutApplyEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &rejectWithoutApplyTx{Tx: tx, err: e.err}, nil
}

func (t *rejectWithoutApplyTx) Commit(ctx context.Context) error {
	t.Discard()
	return t.err
}
