//go:build !js && !wasip1

package s4wave_apt

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	"github.com/s4wave/spacewave/db/testbed"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestAptPackageRootHoldsDeb writes a deb and its package record the way
// ImportDebPackage does, the deb under a stage and the record through a block
// transaction, on an s4db Volume that frees at once. Once the stage is
// released and the collector has run, a named root on the package record is
// all that holds the deb, so the record must name it as an edge.
func TestAptPackageRootHoldsDeb(t *testing.T) {
	// Open a quiet testbed on a Volume that frees at once.
	ctx := t.Context()
	log := logrus.New()
	log.SetLevel(logrus.ErrorLevel)
	tb, err := testbed.NewTestbed(ctx, logrus.NewEntry(log), testbed.WithVolumeConfig(&volume_s4db.Config{
		Path:         filepath.Join(t.TempDir(), "apt.s4wave"),
		VolumeConfig: &volume_controller.Config{GcIntervalDur: "1h"},
		StoreConfig:  &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"},
	}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tb.Release)
	volume, _ := tb.Volume.(*volume_kvtx.Volume)
	if volume == nil {
		t.Fatal("test requires a native s4db Volume")
	}
	cursor, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(cursor.Release)
	store := cursor.GetBucket()

	// Stage the deb as the import does.
	deb := testBusyboxDeb(t)
	stage, releaseStage, err := block.OpenStage(ctx, store)
	if err != nil {
		t.Fatal(err)
	}
	debRef, err := StoreDebPackageBlock(ctx, stage, deb)
	if err != nil {
		t.Fatal(err)
	}

	// Write the completed package record under a named root, then release
	// the stage.
	parsed, err := ParseDebPackage(deb)
	if err != nil {
		t.Fatal(err)
	}
	pkg, err := CompleteAptPackageImport(parsed, debRef)
	if err != nil {
		t.Fatal(err)
	}
	tx, bcs := block.NewTransaction(store, nil, nil, nil)
	bcs.SetBlock(pkg, true)
	root, _, err := tx.Write(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetRoots(ctx, nil, []block.NamedRoot{{Name: "apt:package", Ref: root}}); err != nil {
		t.Fatal(err)
	}
	releaseStage()

	// Collect until no work is left; the package root must keep the deb.
	for done := false; !done; {
		r, err := volume.Collect(ctx)
		if err != nil {
			t.Fatal(err)
		}
		done = !r.More
	}
	for _, c := range []struct {
		name string
		ref  *block.BlockRef
	}{{"package", root}, {"deb", debRef}} {
		found, err := store.GetBlockExists(ctx, c.ref)
		if err != nil {
			t.Fatal(err)
		}
		if !found {
			t.Fatalf("collector freed the %s held by the package root", c.name)
		}
	}
}
