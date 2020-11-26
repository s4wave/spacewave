package volume_test

import (
	"context"

	store_test "github.com/aperturerobotics/hydra/store/test"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// CheckVolume checks a volume with all tests.
func CheckVolume(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
) error {
	return store_test.TestAll(vol)
}
