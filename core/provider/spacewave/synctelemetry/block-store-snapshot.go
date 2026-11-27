package synctelemetry

import "time"

// BlockSource identifies the observed source of a block read.
type BlockSource uint8

const (
	// BlockSourceUnknown means no source has been observed.
	BlockSourceUnknown BlockSource = iota
	// BlockSourceCache means the local upper cache satisfied the read.
	BlockSourceCache
	// BlockSourceDirect means demand-driven DEX satisfied the read.
	BlockSourceDirect
	// BlockSourceCloud means the Cloud packfile baseline satisfied the read.
	BlockSourceCloud
)

// BlockStoreSnapshot describes source and convergence mechanics for one block store.
type BlockStoreSnapshot struct {
	// BlockStoreID identifies the block store.
	BlockStoreID string
	// SharedObjectID is the shared object the store belongs to, if any.
	SharedObjectID string
	// DirectHitCount counts reads served by demand-driven DEX.
	DirectHitCount uint64
	// CloudHitCount counts reads served by the Cloud packfile baseline.
	CloudHitCount uint64
	// CacheHitCount counts reads served by the local upper cache.
	CacheHitCount uint64
	// LastSource is the source of the most recent read.
	LastSource BlockSource
	// AcceptedCheckpointHeight is the height of the last accepted checkpoint.
	AcceptedCheckpointHeight uint64
	// CloudRemoteSequence is the last sequence the Cloud reported.
	CloudRemoteSequence uint64
	// CorruptDirtyCount counts the dirty blocks held back because their local
	// bytes fail their hash. While it is not zero the store publishes nothing.
	CorruptDirtyCount int
	// HeldSince is when the store first held a block back, zero while none is
	// held: nothing has been published from the store since.
	HeldSince time.Time
	// HeldPartial is set when CorruptDirtyCount is a lower bound: the flush
	// that found the blocks stopped before reading every dirty block.
	HeldPartial bool
}
