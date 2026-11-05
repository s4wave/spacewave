//go:build !js

package devtool_store

import (
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_gzip "github.com/s4wave/spacewave/db/block/transform/gzip"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/testbed"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// TestOpenWorldReadsTheControllerWorld checks that the devtool's engine
// controller and OpenWorld open one World: an object the controller commits
// is read by OpenWorld on the same file, since both name its head by EngineID.
func TestOpenWorldReadsTheControllerWorld(t *testing.T) {
	// Open the devtool file through a testbed and its World controller.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(t.TempDir(), FileName)
	tb, err := testbed.NewTestbed(ctx, le, testbed.WithVolumeConfig(&volume_s4db.Config{Path: path}))
	if err != nil {
		t.Fatal(err)
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))
	if _, _, _, err := tb.Volume.ApplyBucketConfig(ctx, &bucket.Config{Id: BucketID, Rev: 1}); err != nil {
		t.Fatal(err)
	}
	transformConf, err := block_transform.NewConfig([]config.Config{&transform_gzip.Config{}})
	if err != nil {
		t.Fatal(err)
	}
	conf := world_block_engine.NewConfig(EngineID, tb.Volume.GetID(), BucketID, ObjectStoreID,
		&bucket.ObjectRef{BucketId: BucketID, TransformConf: transformConf}, nil, false)
	eng, ref, err := world.StartEngine(ctx, tb.Bus, conf, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Commit an object through the controller's World, then close the file.
	ws := world.NewEngineWorldState(eng, true)
	obj, err := ws.CreateObject(ctx, "controller-object", nil)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err)
	}
	ref.Release()
	tb.Release()

	// OpenWorld on the same file reads the object.
	vol, err := OpenVolumeFile(ctx, le, path, false)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	opened, err := OpenWorld(ctx, le, vol, false)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if found, err := world.NewEngineWorldState(opened, false).HasObject(ctx, "controller-object"); err != nil || !found {
		t.Fatalf("OpenWorld missed the controller's object: %v %v", found, err)
	}
}
