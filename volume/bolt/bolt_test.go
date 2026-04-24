//go:build !js && !wasip1

package volume_bolt_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/core"
	"github.com/aperturerobotics/hydra/volume"
	volume_bolt "github.com/aperturerobotics/hydra/volume/bolt"
	volume_test "github.com/aperturerobotics/hydra/volume/test"
	"github.com/sirupsen/logrus"
)

// TestBoltVolume tests the bolt-backed volume including storage stats.
func TestBoltVolume(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(volume_bolt.NewFactory(b))

	tempDir, err := os.MkdirTemp("", "bolt_test_*")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(tempDir)

	path := filepath.Join(tempDir, "test.db")
	volCtrl, _, diRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_bolt.Config{Path: path}),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef.Release()

	bvol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	if err := volume_test.CheckVolume(ctx, bvol); err != nil {
		t.Fatal(err.Error())
	}

	if err := volume_test.CheckStorageStatsNonZero(ctx, bvol); err != nil {
		t.Fatal(err.Error())
	}
}

// TestBoltVolumeSyncsFreelistByDefault verifies bolt volumes use bbolt's
// multi-process-safe freelist mode.
func TestBoltVolumeSyncsFreelistByDefault(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(volume_bolt.NewFactory(b))

	tempDir, err := os.MkdirTemp("", "bolt_test_*")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(tempDir)

	path := filepath.Join(tempDir, "test.db")
	volCtrl, _, diRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&volume_bolt.Config{Path: path}),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef.Release()

	bvol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	bdb := volume_bolt.GetBoltDB(bvol)
	if bdb == nil {
		t.Fatal("expected bolt-backed volume")
	}
	if bdb.NoFreelistSync {
		t.Fatal("expected synced freelist for multi-process bbolt access")
	}
}
