package sobject

import "context"

// SharedObjectReadCheckpointAccessor exposes history retained before read access ended.
// The checkpoint conveys no current membership or write authority.
type SharedObjectReadCheckpointAccessor interface {
	// GetSharedObjectReadCheckpoint returns nil when no history was retained.
	GetSharedObjectReadCheckpoint(ctx context.Context) (*SharedObjectReadCheckpoint, error)
}

// SharedObjectReadCheckpoint records the audience of the World a participant
// last installed before read access ended. Config is descriptive history,
// never current participant authority.
type SharedObjectReadCheckpoint struct {
	// Config records the audience at departure.
	Config *SharedObjectConfig
}
