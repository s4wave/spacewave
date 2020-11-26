package volume_test

import (
	"context"

	kvtx_kvtest "github.com/aperturerobotics/hydra/kvtx/kvtest"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/sirupsen/logrus"
)

// CheckVolume checks a volume with all tests.
func CheckVolume(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
) error {
	if err := CheckObjectStore(ctx, le, vol); err != nil {
		return err
	}

	// TODO: CheckBlockStore, etc
	return nil
}

// CheckObjectStore checks a volume w/ object store test suite.
func CheckObjectStore(
	ctx context.Context,
	le *logrus.Entry,
	vol volume.Volume,
) error {
	id := "test-obj-store"
	objs, err := vol.OpenObjectStore(ctx, id)
	if err != nil {
		return err
	}
	err = kvtx_kvtest.TestAll(ctx, objs)
	if err != nil {
		return err
	}
	return vol.DelObjectStore(ctx, id)
}
