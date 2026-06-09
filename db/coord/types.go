package coord

import "github.com/s4wave/spacewave/db/bucket"

// BackendKind identifies the storage backend behind a coordination capability.
type BackendKind string

const (
	// BackendKindUnknown means the backend was not identified.
	BackendKindUnknown BackendKind = ""
	// BackendKindInMemory identifies the in-memory coordinator.
	BackendKindInMemory BackendKind = "in-memory"
	// BackendKindBbolt identifies the bbolt coordinator adapter.
	BackendKindBbolt BackendKind = "bbolt"
	// BackendKindOPFS identifies the OPFS coordinator adapter.
	BackendKindOPFS BackendKind = "opfs"
	// BackendKindRPC identifies a remote coordinator adapter.
	BackendKindRPC BackendKind = "rpc"
	// BackendKindUnsupported identifies a backend with no direct coordinator.
	BackendKindUnsupported BackendKind = "unsupported"
)

// FallbackReason explains why a caller must use the proxy/RPC fallback path.
type FallbackReason string

const (
	// FallbackReasonNone means no fallback is required.
	FallbackReasonNone FallbackReason = ""
	// FallbackReasonUnsupported means the scoped backend has no coordinator.
	FallbackReasonUnsupported FallbackReason = "unsupported"
	// FallbackReasonDescriptorInvalid means a direct descriptor failed validation.
	FallbackReasonDescriptorInvalid FallbackReason = "descriptor-invalid"
	// FallbackReasonDescriptorStale means the direct descriptor was for an old generation.
	FallbackReasonDescriptorStale FallbackReason = "descriptor-stale"
)

// Scope identifies the coordinated ObjectStore inside a Volume.
type Scope struct {
	// VolumeID is the owning Volume id.
	VolumeID string
	// ObjectStoreID is the ObjectStore id inside the Volume.
	ObjectStoreID string
	// ParticipantID identifies the process or handle performing the operation.
	ParticipantID string
}

// Capability describes direct coordination support for a scoped ObjectStore.
type Capability struct {
	// Supported is true when direct coordination is available.
	Supported bool
	// Backend identifies the backend that would coordinate the scope.
	Backend BackendKind
	// VolumeID is the owning Volume id.
	VolumeID string
	// ObjectStoreID is the ObjectStore id inside the Volume.
	ObjectStoreID string
	// Generation is the latest durable generation observed by the coordinator.
	Generation uint64
	// FallbackReason explains why Supported is false.
	FallbackReason FallbackReason
}

// Snapshot captures the latest durable generation and root metadata.
type Snapshot struct {
	// VolumeID is the owning Volume id.
	VolumeID string
	// ObjectStoreID is the ObjectStore id inside the Volume.
	ObjectStoreID string
	// Generation is the latest durable generation.
	Generation uint64
	// Root is the latest durable ObjectStore root.
	Root *bucket.ObjectRef
}

// Event reports a coordination state change for a scoped ObjectStore.
type Event struct {
	// ProcessID identifies the process that produced or requested the event.
	ProcessID string
	// VolumeID is the owning Volume id.
	VolumeID string
	// ObjectStoreID is the ObjectStore id inside the Volume.
	ObjectStoreID string
	// Generation is the durable generation associated with this event.
	Generation uint64
	// WantLock is true when another participant is waiting for the write lease.
	WantLock bool
	// Unlocked is true when a participant released the write lease.
	Unlocked bool
	// RootChanged carries the accepted root after a commit.
	RootChanged *bucket.ObjectRef
	// KeyPrefixChanged carries a key prefix invalidated by a commit.
	KeyPrefixChanged []byte
	// FallbackReason carries a visible fallback reason when direct coordination fails.
	FallbackReason FallbackReason
}
