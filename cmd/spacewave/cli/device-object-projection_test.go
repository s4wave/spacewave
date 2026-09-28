//go:build !js

package spacewave_cli

import (
	"context"
	"testing"
	"time"

	device_policy "github.com/s4wave/spacewave/core/device/policy"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
)

// staleDeviceEngine rejects the first completed write after its candidate has
// been built, as a concurrent SharedObject root advancement would.
type staleDeviceEngine struct {
	// Engine supplies real World transactions.
	world.Engine
	// attempts counts write candidates opened by the projection.
	attempts int
}

// NewTransaction wraps the first write candidate with a stale commit.
func (e *staleDeviceEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	tx, err := e.Engine.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	e.attempts++
	if e.attempts == 1 {
		return &staleDeviceTx{Tx: tx}, nil
	}
	return tx, nil
}

// staleDeviceTx rejects publication of one otherwise valid candidate.
type staleDeviceTx struct {
	// Tx applies the candidate while Commit injects a stale-base rejection.
	world.Tx
}

// Commit reports that the candidate's base was superseded.
func (t *staleDeviceTx) Commit(context.Context) error {
	return coord.ErrStaleGeneration
}

// TestDeviceObjectProjectionReappliesStaleWrite proves that the Device
// projection reopens the World, rebuilds the object, and commits it.
func TestDeviceObjectProjectionReappliesStaleWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	tb := world_testbed.MustDefault(t, ctx)
	engine := &staleDeviceEngine{Engine: tb.Engine}
	record := &deviceSetupRecord{
		PeerID:     "device-peer",
		Label:      "forge",
		SetupState: deviceSetupStateSessionReady,
	}

	// Apply the enrollment projection across one stale commit.
	key, err := upsertLinkedDeviceObjectInWorld(ctx, engine, record, &device_policy.DevicePolicy{}, time.Now())
	if err != nil {
		t.Fatalf("project Device after stale commit: %v", err)
	}
	if engine.attempts != 2 {
		t.Fatalf("write attempts = %d, want 2", engine.attempts)
	}
	if key != deviceObjectKey(record.PeerID) {
		t.Fatalf("object key = %q, want %q", key, deviceObjectKey(record.PeerID))
	}

	// Read the committed Device from a fresh World transaction.
	tx, err := tb.Engine.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	obj, found, err := tx.GetObject(ctx, key)
	defer world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	if !found {
		t.Fatal("Device object was not committed")
	}
	device, err := readDeviceBlock(ctx, obj)
	if err != nil {
		t.Fatal(err)
	}
	if device.GetPeerId() != record.PeerID || device.GetLabel() != record.Label {
		t.Fatalf("committed Device = %q/%q, want %q/%q", device.GetPeerId(), device.GetLabel(), record.PeerID, record.Label)
	}
}

// _ is a type assertion.
var (
	_ world.Engine = (*staleDeviceEngine)(nil)
	_ world.Tx     = (*staleDeviceTx)(nil)
)
