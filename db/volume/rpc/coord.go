package volume_rpc

import (
	"bytes"

	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/coord"
)

// NewCoordinatorScope converts a coordination scope to its RPC form.
func NewCoordinatorScope(scope coord.Scope) *CoordinatorScope {
	return &CoordinatorScope{
		VolumeId:      scope.VolumeID,
		ObjectStoreId: scope.ObjectStoreID,
		ParticipantId: scope.ParticipantID,
	}
}

// ToCoordScope converts the RPC coordination scope to its local form.
func (x *CoordinatorScope) ToCoordScope() coord.Scope {
	if x == nil {
		return coord.Scope{}
	}
	return coord.Scope{
		VolumeID:      x.GetVolumeId(),
		ObjectStoreID: x.GetObjectStoreId(),
		ParticipantID: x.GetParticipantId(),
	}
}

// NewCoordinatorCapability converts a coordination capability to its RPC form.
func NewCoordinatorCapability(cap *coord.Capability) *CoordinatorCapability {
	if cap == nil {
		return nil
	}
	return &CoordinatorCapability{
		Supported:      cap.Supported,
		Backend:        string(cap.Backend),
		VolumeId:       cap.VolumeID,
		ObjectStoreId:  cap.ObjectStoreID,
		Generation:     cap.Generation,
		FallbackReason: string(cap.FallbackReason),
	}
}

// ToCoordCapability converts the RPC coordination capability to its local form.
func (x *CoordinatorCapability) ToCoordCapability() *coord.Capability {
	if x == nil {
		return nil
	}
	return &coord.Capability{
		Supported:      x.GetSupported(),
		Backend:        coord.BackendKind(x.GetBackend()),
		VolumeID:       x.GetVolumeId(),
		ObjectStoreID:  x.GetObjectStoreId(),
		Generation:     x.GetGeneration(),
		FallbackReason: coord.FallbackReason(x.GetFallbackReason()),
	}
}

// NewCoordinatorEvent converts a coordination event to its RPC form.
func NewCoordinatorEvent(event coord.Event) *CoordinatorEvent {
	return &CoordinatorEvent{
		ProcessId:        event.ProcessID,
		VolumeId:         event.VolumeID,
		ObjectStoreId:    event.ObjectStoreID,
		Generation:       event.Generation,
		WantLock:         event.WantLock,
		Unlocked:         event.Unlocked,
		RootChanged:      cloneObjectRef(event.RootChanged),
		KeyPrefixChanged: bytes.Clone(event.KeyPrefixChanged),
		FallbackReason:   string(event.FallbackReason),
	}
}

// ToCoordEvent converts the RPC coordination event to its local form.
func (x *CoordinatorEvent) ToCoordEvent() coord.Event {
	if x == nil {
		return coord.Event{}
	}
	return coord.Event{
		ProcessID:        x.GetProcessId(),
		VolumeID:         x.GetVolumeId(),
		ObjectStoreID:    x.GetObjectStoreId(),
		Generation:       x.GetGeneration(),
		WantLock:         x.GetWantLock(),
		Unlocked:         x.GetUnlocked(),
		RootChanged:      cloneObjectRef(x.GetRootChanged()),
		KeyPrefixChanged: bytes.Clone(x.GetKeyPrefixChanged()),
		FallbackReason:   coord.FallbackReason(x.GetFallbackReason()),
	}
}

// NewCoordinatorSnapshot converts a coordination snapshot to its RPC form.
func NewCoordinatorSnapshot(snapshot *coord.Snapshot) *CoordinatorSnapshot {
	if snapshot == nil {
		return nil
	}
	return &CoordinatorSnapshot{
		VolumeId:      snapshot.VolumeID,
		ObjectStoreId: snapshot.ObjectStoreID,
		Generation:    snapshot.Generation,
		Root:          cloneObjectRef(snapshot.Root),
	}
}

// ToCoordSnapshot converts the RPC coordination snapshot to its local form.
func (x *CoordinatorSnapshot) ToCoordSnapshot() *coord.Snapshot {
	if x == nil {
		return nil
	}
	return &coord.Snapshot{
		VolumeID:      x.GetVolumeId(),
		ObjectStoreID: x.GetObjectStoreId(),
		Generation:    x.GetGeneration(),
		Root:          cloneObjectRef(x.GetRoot()),
	}
}

func cloneObjectRef(ref *bucket.ObjectRef) *bucket.ObjectRef {
	if ref == nil {
		return nil
	}
	return ref.CloneVT()
}
