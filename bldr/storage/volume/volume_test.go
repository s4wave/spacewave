//go:build !js

package storage_volume

import (
	"context"
	"os"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/volume"
	volume_test "github.com/s4wave/spacewave/db/volume/test"
	"github.com/sirupsen/logrus"
)

func TestStorageDefaultVolume(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(NewFactory(b))

	// build a tmpdir
	tmpDir, err := os.MkdirTemp(os.TempDir(), "bldr-test-*")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(tmpDir)

	// attach the default storage controller
	// this provides separate named volumes with the storage volume controller.
	storageID := default_storage.StorageID
	storageCtrl := default_storage.NewController(storageID, b, tmpDir)
	relStorageCtrl, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer relStorageCtrl()

	// execute the volume
	volCtrlI, _, ref, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&Config{
		StorageId:       storageID,
		StorageVolumeId: "test",
	}), nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ref.Release()

	volCtrl := volCtrlI.(volume.Controller)
	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := volume_test.CheckVolume(ctx, vol); err != nil {
		t.Fatal(err.Error())
	}
}
