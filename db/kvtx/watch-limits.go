package kvtx

import "context"

// WatchLimits bounds one watched snapshot.
// Zero permits an unbounded snapshot.
type WatchLimits struct {
	// MaxRecords is the maximum number of records in one snapshot.
	MaxRecords uint64
	// MaxBytes is the maximum total key and value bytes in one snapshot.
	MaxBytes uint64
}

// BoundedWatchStore streams bounded key/value snapshots after committed changes.
type BoundedWatchStore interface {
	// WatchPrefixBounded calls cb with the current prefix snapshot and each
	// changed snapshot while the snapshot stays within limits.
	// Returns ErrWatchLimit without any callback when a snapshot exceeds limits.
	WatchPrefixBounded(ctx context.Context, prefix []byte, limits WatchLimits, cb func(entries []WatchEntry) error) error
}
