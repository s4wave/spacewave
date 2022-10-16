package volume_test

import (
	"context"

	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/aperturerobotics/hydra/volume"
)

// CheckVolume checks a volume with all store tests.
func CheckVolume(ctx context.Context, vol volume.Volume) error {
	return store_test.TestAll(ctx, vol)
}
