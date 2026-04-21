package volume_sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/volume"
	volume_sqlite "github.com/s4wave/spacewave/db/volume/sqlite"
	volume_test "github.com/s4wave/spacewave/db/volume/test"
	"github.com/sirupsen/logrus"
)

// TestSqliteVolume tests the block graph backed volume.
func TestSqliteVolume(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	sr.AddFactory(volume_sqlite.NewFactory(b))

	// create temporary directory for the test
	tempDir, err := os.MkdirTemp("", "sqlite_test_*")
	if err != nil {
		t.Fatal(err.Error())
	}
	defer os.RemoveAll(tempDir)

	path := filepath.Join(tempDir, "test.db")
	table := "hydra"

	// start the volume
	volCtrl, _, diRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(volume_sqlite.NewConfig(path, table)),
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

	// check volume behavior
	if err := volume_test.CheckVolume(ctx, bvol); err != nil {
		t.Fatal(err.Error())
	}

	// check storage stats return non-zero after writes
	if err := volume_test.CheckStorageStatsNonZero(ctx, bvol); err != nil {
		t.Fatal(err.Error())
	}

	// check volume key
	t.Log(bvol.GetPeerID().String())
}
