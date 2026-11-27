package forge_runtime

import (
	"context"
	"strings"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/world"
	forge_worker "github.com/s4wave/spacewave/forge/worker"
	"github.com/s4wave/spacewave/net/util/confparse"
	s4wave_device "github.com/s4wave/spacewave/sdk/device"
)

// DeviceAvailabilityState describes whether a Device's Worker admits new work.
type DeviceAvailabilityState string

const (
	// DeviceAvailable means the Worker holds an unexpired ACTIVE owner claim.
	DeviceAvailable DeviceAvailabilityState = "available"
	// DeviceDraining means the Worker holds an unexpired DRAINING owner claim.
	DeviceDraining DeviceAvailabilityState = "draining"
	// DeviceUnavailable means the Device has no usable Worker owner claim.
	DeviceUnavailable DeviceAvailabilityState = "unavailable"
)

// DeviceAvailability is a read-only derivation of the Worker's owner claim.
// Self-reported Device liveness and capability state never extend that claim.
type DeviceAvailability struct {
	// State reports whether the Worker can accept new work.
	State DeviceAvailabilityState
	// ClaimEpoch fences the Worker execution that owns the observed claim.
	ClaimEpoch uint64
	// ExpiresAt is the observed owner lease deadline, or zero without a claim.
	ExpiresAt time.Time
	// OwnerState is the observed claim state, including after lease expiry.
	OwnerState CapacityOwnerState
	// Reason explains the derived state.
	Reason string
}

// LookupDeviceAvailability derives availability from the Device's linked Worker
// claim at the current time. A missing Device or claim is unavailable.
func LookupDeviceAvailability(ctx context.Context, ws world.WorldState, deviceObjectKey string) (DeviceAvailability, error) {
	// Read the Device that identifies the Worker whose claim controls availability.
	dev, err := world.LookupObjectBody[s4wave_device.Device](ctx, ws, deviceObjectKey)
	if errors.Is(err, world.ErrObjectNotFound) {
		return DeviceAvailability{State: DeviceUnavailable, Reason: "device not found"}, nil
	}
	if err != nil {
		return DeviceAvailability{}, err
	}

	// Derive from the owner claim without consulting self-reported liveness.
	availability, _, err := LookupForgeWorkerAvailability(ctx, ws, deviceObjectKey, dev)
	return availability, err
}

// LookupForgeWorkerAvailability derives availability for a Device snapshot from
// its linked Worker's claim. The caller may use a read transaction to read both
// the Device and claim at one World revision. It also returns the capacity
// record it derived from, or nil when the Worker has none.
func LookupForgeWorkerAvailability(
	ctx context.Context,
	ws world.WorldState,
	deviceObjectKey string,
	dev *s4wave_device.Device,
) (DeviceAvailability, *WorkerCapacity, error) {
	// Locate the Worker even while its projected capability is draining or stale.
	availability := DeviceAvailability{State: DeviceUnavailable}
	workerKey := ForgeWorkerObjectKey(dev.FindCapabilityByKind(s4wave_device.DeviceCapabilityKindForgeWorker))
	if workerKey == "" {
		availability.Reason = "no forge worker declaration"
		return availability, nil, nil
	}
	capacity, err := LookupWorkerCapacity(ctx, ws, workerKey)
	if errors.Is(err, ErrWorkerNotObserved) {
		availability.Reason = "forge worker has no owner claim"
		return availability, nil, nil
	}
	if err != nil {
		return DeviceAvailability{}, nil, err
	}

	// Preserve claim diagnostics even when the Device cannot accept work.
	availability.ClaimEpoch = capacity.OwnerEpoch
	availability.OwnerState = capacity.OwnerState
	if capacity.OwnerLeaseExpiresAt != nil {
		availability.ExpiresAt = capacity.OwnerLeaseExpiresAt.AsTime()
	}
	if capacity.OwnerDeviceObjectKey != deviceObjectKey {
		availability.Reason = "forge worker claim belongs to device " + capacity.OwnerDeviceObjectKey
		return availability, capacity, nil
	}

	// The Worker claim, not Device liveness, determines expiry and drain.
	switch err := capacity.OwnerClaimActive(time.Now()); {
	case errors.Is(err, ErrCapacityOwnerExpired):
		availability.Reason = "forge worker owner claim expired at " + availability.ExpiresAt.UTC().Format(time.RFC3339)
	case errors.Is(err, ErrCapacityDraining):
		availability.State = DeviceDraining
		availability.Reason = "forge worker owner claim is draining"
	default:
		availability.State = DeviceAvailable
		availability.Reason = "forge worker owner claim is active"
	}
	return availability, capacity, nil
}

// WatchDeviceAvailability emits the initial availability and each changed
// derivation until cancellation or a callback error. ws must be a live World
// state, not a fixed read transaction. World changes wake the watch. The only
// clock is a deadline timer at the current owner lease expiry. It writes
// nothing.
func WatchDeviceAvailability(ctx context.Context, ws world.WorldState, deviceObjectKey string, cb func(DeviceAvailability) error) error {
	// Retain the previous derivation so unrelated World changes emit no update.
	var previous DeviceAvailability
	initial := true
	for {
		// Capture the sequence before reading so concurrent writes cannot be missed.
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			return err
		}
		availability, err := LookupDeviceAvailability(ctx, ws, deviceObjectKey)
		if err != nil {
			return err
		}
		if initial || availability != previous {
			if err := cb(availability); err != nil {
				return err
			}
			previous = availability
			initial = false
		}

		// Wait for a write or the known expiry, replacing the deadline
		// on renewal. A callback may cross the expiry. A past deadline
		// must still wake the watch.
		waitCtx := ctx
		cancel := func() {}
		if availability.State != DeviceUnavailable {
			waitCtx, cancel = context.WithDeadline(ctx, availability.ExpiresAt)
		}
		_, err = ws.WaitSeqno(waitCtx, seqno+1)
		cancel()
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil && !errors.Is(err, context.DeadlineExceeded) {
			return err
		}
	}
}

// SelectForgeWorkerCapability resolves one enrolled Device to its selectable
// Forge Worker capability using the existing typed selection APIs. Device
// selection changes the Worker that runs the same manifest. Selection never
// creates a Device-specific session path or a second scheduler.
func SelectForgeWorkerCapability(dev *s4wave_device.Device) *s4wave_device.DeviceCapability {
	if dev == nil || !dev.IsSelectable() {
		return nil
	}
	return dev.FindSelectableForgeWorker()
}

// ForgeWorkerObjectKey returns the linked Worker object key for a Forge
// Worker capability, empty when the capability has no linked Worker.
func ForgeWorkerObjectKey(capability *s4wave_device.DeviceCapability) string {
	if capability == nil {
		return ""
	}
	return strings.TrimSpace(capability.GetLink().GetObjectKey())
}

// DeviceWorkerPlacement places work on the selectable Forge Worker of dev and
// on dev's own peer, and checks that the peer is linked to that Worker.
func DeviceWorkerPlacement(ctx context.Context, ws world.WorldState, dev *s4wave_device.Device) (*forge_worker.Placement, error) {
	// Require a selectable Device with a linked Worker.
	workerKey := ForgeWorkerObjectKey(SelectForgeWorkerCapability(dev))
	if workerKey == "" {
		return nil, errors.New("device has no available Forge worker")
	}

	// Bind the Device's peer to the Worker it links.
	devicePeer, err := confparse.ParsePeerID(dev.GetPeerId())
	if err != nil {
		return nil, err
	}
	placement := &forge_worker.Placement{WorkerObjectKey: workerKey, PeerId: devicePeer.String()}
	if err := placement.ValidateLinked(ctx, ws); err != nil {
		return nil, err
	}
	return placement, nil
}
