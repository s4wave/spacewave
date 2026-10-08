//go:build !js

package spacewave_cli

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
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
	// Fail the first write transaction, then delegate.
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
	// Open a testbed World and a stale engine with a ready setup record.
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
	key, err := upsertLinkedDeviceObjectInWorld(ctx, engine, record, time.Now(), nil)
	if err != nil {
		t.Fatalf("project Device after stale commit: %v", err)
	}
	if engine.attempts != 2 {
		t.Fatalf("write attempts = %d, want 2", engine.attempts)
	}
	if key != deviceObjectKey(record.PeerID) {
		t.Fatalf("object key = %q, want %q", key, deviceObjectKey(record.PeerID))
	}
	requireCommittedDevice(t, ctx, tb.Engine, key, record)
}

// blockedDeviceEngine refuses writes while blocked, as a World whose replay
// waits for an operation's missing block does.
type blockedDeviceEngine struct {
	// Engine supplies real World transactions.
	world.Engine
	// blocked rejects write transactions while set.
	blocked atomic.Bool
}

// NewTransaction rejects a write while the engine is blocked.
func (e *blockedDeviceEngine) NewTransaction(ctx context.Context, write bool) (world.Tx, error) {
	if write && e.blocked.Load() {
		return nil, block.ErrNotFound
	}
	return e.Engine.NewTransaction(ctx, write)
}

// TestDeviceObjectProjectionWaitsForWorld proves that a waiting projection
// retries a refused write once the World advances, instead of failing.
func TestDeviceObjectProjectionWaitsForWorld(t *testing.T) {
	// Open a testbed World that refuses writes.
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	tb := world_testbed.MustDefault(t, ctx)
	engine := &blockedDeviceEngine{Engine: tb.Engine}
	engine.blocked.Store(true)
	record := &deviceSetupRecord{
		PeerID:     "device-peer",
		Label:      "forge",
		SetupState: deviceSetupStateSessionReady,
	}

	// On the first refusal, unblock and advance the World from another writer.
	advanced := make(chan error, 1)
	blocked := 0
	onBlocked := func(error) {
		blocked++
		if blocked != 1 {
			return
		}
		go func() {
			engine.blocked.Store(false)
			advanced <- writeUnrelatedObject(ctx, tb.Engine)
		}()
	}
	key, err := upsertLinkedDeviceObjectInWorld(ctx, engine, record, time.Now(), onBlocked)
	if err != nil {
		t.Fatalf("project Device after the World advanced: %v", err)
	}
	if err := <-advanced; err != nil {
		t.Fatal(err)
	}
	if blocked != 1 {
		t.Fatalf("blocked reports = %d, want 1", blocked)
	}

	// Require the Device committed after the World advanced.
	requireCommittedDevice(t, ctx, tb.Engine, key, record)
}

// writeUnrelatedObject commits one object to advance the World.
func writeUnrelatedObject(ctx context.Context, engine world.Engine) error {
	// Open a write transaction.
	tx, err := engine.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Create the object and commit it.
	obj, _, err := world.CreateWorldObject(ctx, tx, "other", func(bcs *block.Cursor) error {
		bcs.SetBlock(&s4wave_device.Device{PeerId: "other"}, true)
		return nil
	})
	world.ReleaseObjectState(obj)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// requireCommittedDevice reads the Device at key from a fresh World
// transaction and requires the record's peer, label, and setup state.
func requireCommittedDevice(t *testing.T, ctx context.Context, engine world.Engine, key string, record *deviceSetupRecord) {
	// Read the committed Device from a fresh World transaction.
	t.Helper()
	tx, err := engine.NewTransaction(ctx, false)
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

	// Require the committed Device peer, label, and setup state.
	device, err := readDeviceBlock(ctx, obj)
	if err != nil {
		t.Fatal(err)
	}
	if device.GetPeerId() != record.PeerID || device.GetLabel() != record.Label {
		t.Fatalf("committed Device = %q/%q, want %q/%q", device.GetPeerId(), device.GetLabel(), record.PeerID, record.Label)
	}
	if device.GetSetupState() != deviceSetupStateProto(record.SetupState) {
		t.Fatalf("committed setup state = %v, want %v", device.GetSetupState(), deviceSetupStateProto(record.SetupState))
	}
}

// _ is a type assertion.
var (
	_ world.Engine = (*staleDeviceEngine)(nil)
	_ world.Engine = (*blockedDeviceEngine)(nil)
	_ world.Tx     = (*staleDeviceTx)(nil)
)
