package forge_runtime

import (
	"context"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/world"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// testSelectableDevice declares a ready Device with filesystem and Worker capabilities.
func testSelectableDevice() *s4wave_device.Device {
	return &s4wave_device.Device{
		PeerId:     "12D3KooWPeer",
		Label:      "bench-1",
		SetupState: s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_DEVICE_SESSION_READY,
		Capabilities: []*s4wave_device.DeviceCapability{
			{
				Id:    "cap-fs",
				Kind:  s4wave_device.DeviceCapabilityKindFilesystem,
				State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			},
			{
				Id:    "cap-worker",
				Kind:  s4wave_device.DeviceCapabilityKindForgeWorker,
				State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_ACTIVE,
				Link: &s4wave_device.DeviceCapabilityLink{
					ObjectKey: "worker/bench-1",
					TypeId:    "forge/worker",
				},
			},
		},
	}
}

// TestSelectForgeWorkerCapabilityUsesTypedSelection preserves typed Worker selection.
func TestSelectForgeWorkerCapabilityUsesTypedSelection(t *testing.T) {
	// Select the Forge Worker capability from a ready Device.
	dev := testSelectableDevice()
	capability := SelectForgeWorkerCapability(dev)
	if capability == nil || ForgeWorkerObjectKey(capability) != "worker/bench-1" {
		t.Fatalf("unexpected capability: %+v", capability)
	}

	// Check that a Device awaiting setup cannot supply a Worker.
	notReady := testSelectableDevice()
	notReady.SetupState = s4wave_device.DeviceSetupState_DEVICE_SETUP_STATE_WAITING_FOR_COMPLETION
	if SelectForgeWorkerCapability(notReady) != nil {
		t.Fatal("non-selectable device must not resolve a worker")
	}

	// Check that a Device without a Worker capability cannot supply one.
	noWorker := testSelectableDevice()
	noWorker.Capabilities = noWorker.Capabilities[:1]
	if SelectForgeWorkerCapability(noWorker) != nil {
		t.Fatal("device without forge-worker capability must not resolve a worker")
	}

	// Check that an absent Device cannot supply a Worker.
	if SelectForgeWorkerCapability(nil) != nil {
		t.Fatal("nil device must not resolve a worker")
	}
}

// TestDeviceAvailabilityDrainAndDeclarationRemoval follows the Worker's drain
// without relying on projected capability state or self-reported liveness.
func TestDeviceAvailabilityDrainAndDeclarationRemoval(t *testing.T) {
	// Publish an ONLINE Device and start a watch before its Worker claims capacity.
	ctx, eng, _ := newTestbed(t)
	dev := availabilityTestDevice()
	publishAvailabilityTestDevice(t, ctx, eng, dev)
	ws := world.NewEngineWorldState(eng, false)
	updates := watchAvailabilityForTest(t, ctx, ws)
	if got := nextAvailabilityForTest(t, updates); got.State != DeviceUnavailable || got.Reason != "forge worker has no owner claim" {
		t.Fatalf("unclaimed Device = %+v", got)
	}

	// An ACTIVE claim makes the Device available at the Worker's committed epoch.
	admission := NewWorldRuntimeAdmission(eng, nil, 0, time.Minute)
	claim, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	active := nextAvailabilityForTest(t, updates)
	if active.State != DeviceAvailable || active.ClaimEpoch != claim.OwnerEpoch ||
		active.OwnerState != CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE || !active.ExpiresAt.Equal(claim.OwnerLeaseExpiresAt.AsTime()) {
		t.Fatalf("active claim = %+v", active)
	}

	// Draining changes availability while the projected capability remains AVAILABLE.
	if _, err := admission.BeginDrainCapacity(ctx, "worker/a", selfRef, claim.OwnerEpoch); err != nil {
		t.Fatal(err)
	}
	draining := nextAvailabilityForTest(t, updates)
	if draining.State != DeviceDraining || draining.OwnerState != CapacityOwnerState_CAPACITY_OWNER_STATE_DRAINING ||
		draining.ClaimEpoch != active.ClaimEpoch || draining.Reason != "forge worker owner claim is draining" {
		t.Fatalf("draining claim = %+v", draining)
	}

	// Removing the declaration makes the Device unavailable even with a live claim.
	dev.Capabilities = nil
	publishAvailabilityTestDevice(t, ctx, eng, dev)
	removed := nextAvailabilityForTest(t, updates)
	if removed.State != DeviceUnavailable || removed.ClaimEpoch != 0 || !removed.ExpiresAt.IsZero() ||
		removed.OwnerState != CapacityOwnerState_CAPACITY_OWNER_STATE_UNKNOWN || removed.Reason != "no forge worker declaration" {
		t.Fatalf("removed declaration = %+v", removed)
	}
}

// TestDeviceAvailabilityExpiryAndNewEpoch proves the watch expires an ONLINE
// Device without a World write and observes a replacement Worker claim.
func TestDeviceAvailabilityExpiryAndNewEpoch(t *testing.T) {
	// Give an ONLINE Device a short-lived Worker claim in a real in-memory World.
	ctx, eng, _ := newTestbed(t)
	publishAvailabilityTestDevice(t, ctx, eng, availabilityTestDevice())
	admission := NewWorldRuntimeAdmission(eng, nil, 0, time.Second)
	claim, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	ws := world.NewEngineWorldState(eng, false)
	updates := watchAvailabilityForTest(t, ctx, ws)
	if got := nextAvailabilityForTest(t, updates); got.State != DeviceAvailable {
		t.Fatalf("initial claim = %+v", got)
	}

	// A renewal within the lease replaces the watch deadline without changing
	// the owner epoch.
	renewalTime := time.Now().Add(time.Second / 2)
	admission.SetTimeNow(func() time.Time { return renewalTime })
	renewal, err := admission.RenewWorkerClaim(ctx, "worker/a", selfRef)
	admission.SetTimeNow(time.Now)
	if err != nil {
		t.Fatal(err)
	}
	if renewal.OwnerEpoch != claim.OwnerEpoch {
		t.Fatalf("renewal epoch = %d, want %d", renewal.OwnerEpoch, claim.OwnerEpoch)
	}
	renewed := nextAvailabilityForTest(t, updates)
	if renewed.State != DeviceAvailable || renewed.ClaimEpoch != renewal.OwnerEpoch ||
		!renewed.ExpiresAt.Equal(renewal.OwnerLeaseExpiresAt.AsTime()) {
		t.Fatalf("renewed claim = %+v", renewed)
	}

	// The known deadline alone emits unavailable while retaining claim diagnostics.
	before, err := eng.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	expired := nextAvailabilityForTest(t, updates)
	if expired.State != DeviceUnavailable || expired.Reason != "forge worker owner claim expired at "+expired.ExpiresAt.UTC().Format(time.RFC3339) ||
		expired.ClaimEpoch != renewal.OwnerEpoch || expired.OwnerState != CapacityOwnerState_CAPACITY_OWNER_STATE_ACTIVE ||
		!expired.ExpiresAt.Equal(renewal.OwnerLeaseExpiresAt.AsTime()) || time.Now().Before(expired.ExpiresAt) {
		t.Fatalf("expired claim = %+v", expired)
	}

	// Expiry is read-only and does not rewrite the Device's ONLINE diagnostic.
	after, err := eng.GetSeqno(ctx)
	if err != nil {
		t.Fatal(err)
	}
	dev, err := world.LookupObjectBody[s4wave_device.Device](ctx, ws, selfRef.DeviceObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if before != after || dev.GetLastStatus().GetLiveness() != s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE {
		t.Fatal("expiry wrote World state or changed self-reported liveness")
	}

	// A new Worker execution restores availability at a strictly higher epoch.
	replacement := WorkerClaimRef{DeviceObjectKey: selfRef.DeviceObjectKey, ClaimID: "replacement"}
	newClaim, err := admission.ClaimWorkerCapacity(ctx, "worker/a", replacement)
	if err != nil {
		t.Fatal(err)
	}
	restored := nextAvailabilityForTest(t, updates)
	if restored.State != DeviceAvailable || restored.ClaimEpoch != newClaim.OwnerEpoch ||
		restored.ClaimEpoch <= expired.ClaimEpoch || !restored.ExpiresAt.After(expired.ExpiresAt) {
		t.Fatalf("replacement claim = %+v", restored)
	}
}

// TestDeviceAvailabilityNewEpochAfterCompletedDrain proves a claim made after a
// completed drain deleted the record is distinguishable from the drained one.
func TestDeviceAvailabilityNewEpochAfterCompletedDrain(t *testing.T) {
	// Claim the Worker, then drain it until the capacity record is deleted.
	ctx, eng, _ := newTestbed(t)
	publishAvailabilityTestDevice(t, ctx, eng, availabilityTestDevice())
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), 0, time.Minute)
	claim, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	ws := world.NewEngineWorldState(eng, false)

	// Replace the claim, then drain the replacement.
	drained := WorkerClaimRef{DeviceObjectKey: selfRef.DeviceObjectKey, ClaimID: "drained"}
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", drained); err != nil {
		t.Fatal(err)
	}
	if err := admission.DrainWorker(ctx, "worker/a", drained, claim.OwnerEpoch+1); err != nil {
		t.Fatal(err)
	}

	// The drain deleted the capacity record.
	gone, err := LookupDeviceAvailability(ctx, ws, selfRef.DeviceObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if gone.State != DeviceUnavailable || gone.Reason != "forge worker has no owner claim" {
		t.Fatalf("drained Device = %+v", gone)
	}

	// A fresh claim a moment later reports an epoch above every epoch before the drain.
	later := time.Now().Add(time.Second)
	admission.SetTimeNow(func() time.Time { return later })
	fresh, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef)
	if err != nil {
		t.Fatal(err)
	}
	restored, err := LookupDeviceAvailability(ctx, ws, selfRef.DeviceObjectKey)
	if err != nil {
		t.Fatal(err)
	}
	if restored.State != DeviceAvailable || restored.ClaimEpoch != fresh.OwnerEpoch || restored.ClaimEpoch <= claim.OwnerEpoch+1 {
		t.Fatalf("claim after drain = %+v, drained epoch %d", restored, claim.OwnerEpoch+1)
	}
}

// TestDeviceAvailabilityCallbackCrossesExpiry prevents losing a deadline when
// delivering the previous observation takes longer than the remaining lease.
func TestDeviceAvailabilityCallbackCrossesExpiry(t *testing.T) {
	// Start an ONLINE Device with a live claim whose deadline will cross the callback.
	ctx, eng, _ := newTestbed(t)
	publishAvailabilityTestDevice(t, ctx, eng, availabilityTestDevice())
	admission := NewWorldRuntimeAdmission(eng, nil, 0, time.Second)
	if _, err := admission.ClaimWorkerCapacity(ctx, "worker/a", selfRef); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	// Hold the first delivery until expiry, then require an expired observation.
	stop := errors.New("observed expiry")
	err := WatchDeviceAvailability(ctx, world.NewEngineWorldState(eng, false), selfRef.DeviceObjectKey, func(got DeviceAvailability) error {
		switch got.State {
		case DeviceAvailable:
			// Delay only until this claim's known expiry, never a polling interval.
			timer := time.NewTimer(time.Until(got.ExpiresAt))
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		case DeviceUnavailable:
			if got.Reason != "forge worker owner claim expired at "+got.ExpiresAt.UTC().Format(time.RFC3339) {
				t.Fatalf("unexpected unavailable observation: %+v", got)
			}
			return stop
		default:
			t.Fatalf("unexpected claim state: %+v", got)
			return nil
		}
	})
	if !errors.Is(err, stop) {
		t.Fatalf("watch did not deliver expiry after callback: %v", err)
	}
}

// availabilityTestDevice declares a Worker whose projected state remains available.
func availabilityTestDevice() *s4wave_device.Device {
	return &s4wave_device.Device{
		PeerId: "peer-device",
		Label:  "Build host",
		LastStatus: &s4wave_device.DeviceStatus{
			Liveness: s4wave_device.DeviceLiveness_DEVICE_LIVENESS_ONLINE,
		},
		Capabilities: []*s4wave_device.DeviceCapability{{
			Id:    "forge-worker",
			Kind:  s4wave_device.DeviceCapabilityKindForgeWorker,
			State: s4wave_device.DeviceCapabilityState_DEVICE_CAPABILITY_STATE_AVAILABLE,
			Link: &s4wave_device.DeviceCapabilityLink{
				ObjectKey: "worker/a",
				TypeId:    "forge/worker",
			},
		}},
	}
}

// publishAvailabilityTestDevice writes only the fixture Device in the test World.
func publishAvailabilityTestDevice(t *testing.T, ctx context.Context, eng world.Engine, dev *s4wave_device.Device) {
	// Commit the fixture through the same World transaction API used by projection.
	t.Helper()
	err := world.ExecTransaction(ctx, eng, true, func(ctx context.Context, ws world.WorldState) error {
		_, _, err := world.AccessWorldObject(ctx, ws, selfRef.DeviceObjectKey, true, func(cursor *block.Cursor) error {
			cursor.SetBlock(dev, true)
			return nil
		})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
}

// watchAvailabilityForTest runs a watch and joins it before the test World closes.
func watchAvailabilityForTest(t *testing.T, ctx context.Context, ws world.WorldState) <-chan DeviceAvailability {
	// Isolate watch cancellation and retain its completion for cleanup.
	t.Helper()
	ctx, cancel := context.WithCancel(ctx)
	updates := make(chan DeviceAvailability)
	done := make(chan error, 1)
	go func() {
		done <- WatchDeviceAvailability(ctx, ws, selfRef.DeviceObjectKey, func(got DeviceAvailability) error {
			select {
			case updates <- got:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		})
	}()

	// Stop delivery before releasing the engine and report unexpected watch failures.
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("availability watch: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("availability watch did not stop")
		}
	})
	return updates
}

// nextAvailabilityForTest waits for one pushed observation with a failure deadline.
func nextAvailabilityForTest(t *testing.T, updates <-chan DeviceAvailability) DeviceAvailability {
	t.Helper()
	select {
	case got := <-updates:
		return got
	case <-time.After(5 * time.Second):
		t.Fatal("availability observation did not arrive")
		return DeviceAvailability{}
	}
}
