package resource_world

import (
	"time"

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
	// BlockReadCount is the number of block cursor fetches issued by the operation.
	BlockReadCount uint64
	// BlockReadBytes is the number of block bytes returned to block cursors.
	BlockReadBytes uint64
	// BlockReadMissCount is the number of block cursor fetches that missed.
	BlockReadMissCount uint64
	// ResourceGetBlockCount is the number of Resource SDK GetBlock calls.
	ResourceGetBlockCount uint64
	// ResourceGetBlockRefCount is the number of non-empty refs passed to Resource SDK GetBlock.
	ResourceGetBlockRefCount uint64
	// ResourceGetBlockBytes is the number of bytes returned by found Resource SDK GetBlock calls.
	ResourceGetBlockBytes uint64
	// ResourceGetBlockMissCount is the number of Resource SDK GetBlock calls that missed.
	ResourceGetBlockMissCount uint64
	// DecodedBlockUnmarshalCount is the number of decoded block unmarshals.
	DecodedBlockUnmarshalCount uint64
	// DecodedBlockUnmarshalBytes is the number of bytes passed to decoded block unmarshalling.
	DecodedBlockUnmarshalBytes uint64
	// DecodedBlockCacheAttemptCount is the number of decoded-block cache lookup attempts.
	DecodedBlockCacheAttemptCount uint64
	// DecodedBlockCacheHitCount is the number of decoded-block cache hits.
	DecodedBlockCacheHitCount uint64
	// DecodedBlockCacheMissCount is the number of decoded-block cache misses.
	DecodedBlockCacheMissCount uint64
	// DecodedBlockCloneCount is the number of clone-on-hit decoded block returns.
	DecodedBlockCloneCount uint64
	// DecodedBlockUncloneableCount is the number of cache candidates that could not be cloned.
	DecodedBlockUncloneableCount uint64
	// DecodedBlockUncacheableCount is the number of decoded-block cache bypasses for missing exact keys.
	DecodedBlockUncacheableCount uint64
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
