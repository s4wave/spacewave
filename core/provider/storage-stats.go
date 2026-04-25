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
