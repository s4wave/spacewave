package volume_test

import (
	"context"

	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/pkg/errors"
)

// CheckVolume checks a volume with all store tests.
func CheckVolume(ctx context.Context, vol volume.Volume) error {
	if err := store_test.TestAll(ctx, vol); err != nil {
		return err
	}
	return CheckStorageStats(ctx, vol)
}

// CheckStorageStats verifies GetStorageStats returns without error.
func CheckStorageStats(ctx context.Context, vol volume.Volume) error {
	_, err := vol.GetStorageStats(ctx)
	return err
}

// CheckStorageStatsNonZero verifies GetStorageStats returns non-zero
// TotalBytes and BlockCount. Call after writing data to the volume.
func CheckStorageStatsNonZero(ctx context.Context, vol volume.Volume) error {
	stats, err := vol.GetStorageStats(ctx)
	if err != nil {
		return err
	}
	if stats.GetTotalBytes() == 0 {
		return errors.New("expected non-zero TotalBytes after writing data")
	}
	if stats.GetBlockCount() == 0 {
		return errors.New("expected non-zero BlockCount after writing data")
	}
	return nil
}
