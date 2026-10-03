package volume_sqlite_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/s4wave/spacewave/db/coord"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/volume"
	volume_sqlite "github.com/s4wave/spacewave/db/volume/sqlite"
	volume_test "github.com/s4wave/spacewave/db/volume/test"
	"github.com/sirupsen/logrus"
)

// TestSqliteVolume tests the block graph backed volume.
func TestSqliteVolume(t *testing.T) {
	// Configure debug logging for the SQLite volume test.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Register the SQLite volume factory on the test storage bus.
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

	// Select a database path and table within the temporary test directory.
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

	// Obtain the running SQLite volume for the shared volume contract checks.
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

func TestSqliteKeyedLeaseNamespacesTables(t *testing.T) {
	// Prepare a shared database path for two independent volume tables.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(t.TempDir(), "shared.db")

	// Open the first table as a SQLite volume.
	volumeA, err := volume_sqlite.NewSqlite(ctx, le, volume_sqlite.NewConfig(path, "hydra_a"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = volumeA.Close() })

	// Open the second table in the same SQLite database.
	volumeB, err := volume_sqlite.NewSqlite(ctx, le, volume_sqlite.NewConfig(path, "hydra_b"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = volumeB.Close() })

	// Require the first table to acquire its shared-object write lease.
	leaseA, acquired, err := volumeA.TryAcquireWriteLease(ctx, coord.Scope{VolumeID: volumeA.GetID(), Key: "shared-object"})
	if err != nil || !acquired {
		t.Fatalf("acquire first SQLite table lease: acquired=%v err=%v", acquired, err)
	}
	t.Cleanup(func() { _ = leaseA.Release(ctx) })

	// Verify the second table can lease the same object key independently.
	leaseB, acquired, err := volumeB.TryAcquireWriteLease(ctx, coord.Scope{VolumeID: volumeB.GetID(), Key: "shared-object"})
	if err != nil || !acquired {
		t.Fatalf("acquire second SQLite table lease: acquired=%v err=%v", acquired, err)
	}
	t.Cleanup(func() { _ = leaseB.Release(ctx) })
}
