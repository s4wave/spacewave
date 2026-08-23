package resource_world

import (
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
)

// WorldStateOperationRecord describes one WorldStateResource operation.
type WorldStateOperationRecord struct {
	// Name is the WorldStateResource method name.
	Name string
	// Duration is the elapsed wall-clock duration.
	Duration time.Duration
	// Error is set when the operation returned an error.
	Error string
	// FilterCount is the number of graph filters in the request.
	FilterCount int
	// Limit is the relevant request limit.
	Limit int
	// ResultSetCount is the number of result groups returned.
	ResultSetCount int
	// ResultQuadCount is the total number of graph quads returned.
	ResultQuadCount int
	// StartKeyCount is the number of graph path start keys.
	StartKeyCount int
	// StepCount is the number of graph path steps.
	StepCount int
	// PageSize is the requested page size for resource-backed operations.
	PageSize int
	// ResultObjectCount is the number of object keys returned by the operation.
	ResultObjectCount int
	// ResourceCreated indicates that the operation attached a resource.
	ResourceCreated bool
	// ReadCounterSnapshot is the block read-counter snapshot for the operation.
	ReadCounterSnapshot block.ReadCounterSnapshot
}

// WorldStateOperationObserver observes WorldStateResource operation records.
type WorldStateOperationObserver func(WorldStateOperationRecord)

// WorldStateResourceOption configures a WorldStateResource.
type WorldStateResourceOption func(*WorldStateResource)

// WithWorldStateOperationObserver configures WorldStateResource operation accounting.
func WithWorldStateOperationObserver(observer WorldStateOperationObserver) WorldStateResourceOption {
	return func(r *WorldStateResource) {
		r.operationObserver = observer
	}
}

// WithSessionPeerID configures the trusted session peer for typed object access.
func WithSessionPeerID(sessionPeerID peer.ID) WorldStateResourceOption {
	return func(r *WorldStateResource) {
		r.sessionPeerID = sessionPeerID
		r.sessionPeerIDBound = true
	}
}

func applyWorldStateResourceOptions(r *WorldStateResource, opts ...WorldStateResourceOption) {
	for _, opt := range opts {
		if opt != nil {
			opt(r)
		}
	}
}

func worldStateResourceSessionPeerID(opts ...WorldStateResourceOption) (peer.ID, bool) {
	r := new(WorldStateResource)
	applyWorldStateResourceOptions(r, opts...)
	return r.sessionPeerID, r.sessionPeerIDBound
}
