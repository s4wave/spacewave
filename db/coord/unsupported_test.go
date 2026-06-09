package coord

import (
	"context"
	"errors"
	"testing"
)

func TestUnsupportedCoordinatorReportsFallbackCapability(t *testing.T) {
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
	if capability.Backend != BackendKindUnsupported {
		t.Fatalf("unexpected backend: %q", capability.Backend)
	}
	if capability.VolumeID != "volume-a" {
		t.Fatalf("unexpected volume id: %q", capability.VolumeID)
	}
	if capability.ObjectStoreID != "objects" {
		t.Fatalf("unexpected object store id: %q", capability.ObjectStoreID)
	}
	if capability.Generation != 0 {
		t.Fatalf("unexpected generation: %d", capability.Generation)
	}
	if capability.FallbackReason != FallbackReasonUnsupported {
		t.Fatalf("unexpected fallback reason: %q", capability.FallbackReason)
	}
}

func TestUnsupportedCoordinatorRejectsCoordinationOps(t *testing.T) {
	coord := NewUnsupportedCoordinator(BackendKindUnknown, FallbackReasonNone)
	scope := Scope{
		VolumeID:      "volume-a",
		ObjectStoreID: "objects",
		ParticipantID: "process-a",
	}

	if snapshot, err := coord.Snapshot(context.Background(), scope); !errors.Is(err, ErrUnsupported) || snapshot != nil {
		t.Fatalf("Snapshot() = (%v, %v), want nil ErrUnsupported", snapshot, err)
	}
	if watch, err := coord.Watch(context.Background(), scope, 0); !errors.Is(err, ErrUnsupported) || watch != nil {
		t.Fatalf("Watch() = (%v, %v), want nil ErrUnsupported", watch, err)
	}
	if lease, ok, err := coord.TryAcquireWriteLease(context.Background(), scope); !errors.Is(err, ErrUnsupported) || lease != nil || ok {
		t.Fatalf("TryAcquireWriteLease() = (%v, %v, %v), want nil false ErrUnsupported", lease, ok, err)
	}
	if lease, err := coord.WaitAcquireWriteLease(context.Background(), scope); !errors.Is(err, ErrUnsupported) || lease != nil {
		t.Fatalf("WaitAcquireWriteLease() = (%v, %v), want nil ErrUnsupported", lease, err)
	}
}
