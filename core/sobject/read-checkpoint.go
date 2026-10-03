package sobject

import "context"

// SharedObjectReadCheckpointAccessor exposes history retained before read access ended.
// The checkpoint conveys no current membership or write authority.
type SharedObjectReadCheckpointAccessor interface {
	// GetSharedObjectReadCheckpoint returns nil when no history was retained.
	GetSharedObjectReadCheckpoint(ctx context.Context) (*SharedObjectReadCheckpoint, error)
}

// SharedObjectReadCheckpoint records the state a participant last held before
// read access ended. It is descriptive history, never current participant
// authority.
type SharedObjectReadCheckpoint struct {
	// Config records the audience at departure.
	Config *SharedObjectConfig
	// Snapshot is the state held at departure. Replaying it yields the World
	// the participant last could read.
	Snapshot SharedObjectStateSnapshot
}
