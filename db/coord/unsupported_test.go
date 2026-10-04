package coord

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedCoordinatorReportsFallbackCapability(t *testing.T) {
	// Request capability for a specific volume, store, and participant.
	coord := NewUnsupportedCoordinator(BackendKindUnsupported, FallbackReasonUnsupported)
	capability, err := coord.Capability(context.Background(), Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	})
	if err != nil {
		t.Fatal(err)
	}
	if capability.Supported {
		t.Fatal("unsupported coordinator reported supported capability")
	}

	// Verify the fallback retains its backend and resource identities.
	if capability.Backend != BackendKindUnsupported {
		t.Fatalf("unexpected backend: %q", capability.Backend)
	}
	if capability.VolumeID != "volume-a" {
		t.Fatalf("unexpected volume id: %q", capability.VolumeID)
	}
	if capability.ObjectStoreID != "objects" {
		t.Fatalf("unexpected object store id: %q", capability.ObjectStoreID)
	}

	// Confirm unsupported capability has no generation and names its fallback.
	if capability.Generation != 0 {
		t.Fatalf("unexpected generation: %d", capability.Generation)
	}
	if capability.FallbackReason != FallbackReasonUnsupported {
		t.Fatalf("unexpected fallback reason: %q", capability.FallbackReason)
	}
}

func TestUnsupportedCoordinatorRejectsCoordinationOps(t *testing.T) {
	// Prepare one scope for each unsupported coordination operation.
	coord := NewUnsupportedCoordinator(BackendKindUnknown, FallbackReasonNone)
	scope := Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}

	// Reject snapshot and watch requests for the unsupported backend.
	if snapshot, err := coord.Snapshot(context.Background(), scope); !errors.Is(err, ErrUnsupported) || snapshot != nil {
		t.Fatalf("Snapshot() = (%v, %v), want nil ErrUnsupported", snapshot, err)
	}
	if watch, err := coord.Watch(context.Background(), scope, 0); !errors.Is(err, ErrUnsupported) || watch != nil {
		t.Fatalf("Watch() = (%v, %v), want nil ErrUnsupported", watch, err)
	}

	// Reject both nonblocking and waiting write-lease requests.
	if lease, ok, err := coord.TryAcquireWriteLease(context.Background(), scope); !errors.Is(err, ErrUnsupported) || lease != nil || ok {
		t.Fatalf("TryAcquireWriteLease() = (%v, %v, %v), want nil false ErrUnsupported", lease, ok, err)
	}
	if lease, err := coord.WaitAcquireWriteLease(context.Background(), scope); !errors.Is(err, ErrUnsupported) || lease != nil {
		t.Fatalf("WaitAcquireWriteLease() = (%v, %v), want nil ErrUnsupported", lease, err)
	}
}
