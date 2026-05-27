package provider

import (
	"context"

	"github.com/s4wave/spacewave/db/volume"
)

// StorageStatsProvider is an optional interface that ProviderAccount
// implementations can satisfy to report account-level storage statistics.
type StorageStatsProvider interface {
	// GetStorageStats returns storage usage statistics for the account volume.
	GetStorageStats(ctx context.Context) (*volume.StorageStats, error)
}

// StorageStatsWatchProvider is an optional interface that ProviderAccount
// implementations can satisfy to expose a storage-stats snapshot and a channel
// that closes when storage usage may have changed.
type StorageStatsWatchProvider interface {
	StorageStatsProvider

	// GetStorageStatsSnapshotWithWait returns current storage stats and a wait
	// channel that closes after the next storage stats mutation.
	GetStorageStatsSnapshotWithWait(ctx context.Context) (*volume.StorageStats, <-chan struct{}, error)
}
