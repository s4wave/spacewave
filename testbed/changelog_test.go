package testbed

import (
	"testing"

	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/db/bucket"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
)

// TestChangelogStorageVolume checks that a new World with the changelog
// enabled commits a clean write and then a batch of writes on an s4db storage
// volume.
func TestChangelogStorageVolume(t *testing.T) {
	// Start a testbed on s4db storage.
	ctx := t.Context()
	tb, err := Default(ctx, WithStorages(storage_native.NewS4db(false, t.TempDir())))
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Start an s4db volume with a World bucket.
	volCtrl, volRef, err := storage_volume.ExecVolumeController(ctx, tb.Bus, &storage_volume.Config{
		StorageId:       tb.StorageID,
		StorageVolumeId: "changelog-world",
		VolumeConfig:    &volume_controller.Config{},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer volRef.Release()
	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err)
	}
	const bucketID = "changelog-world-bucket"
	if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: bucketID, Rev: 1}); err != nil {
		t.Fatal(err)
	}

	// Start a World engine with the changelog on the volume.
	transformConf, err := newEngineTransformConfig(bucketID)
	if err != nil {
		t.Fatal(err)
	}
	conf := world_block_engine.NewConfig(
		"changelog-world",
		vol.GetID(),
		bucketID,
		"changelog-world-store",
		&bucket.ObjectRef{BucketId: bucketID, TransformConf: transformConf},
		nil,
		true,
	)
	ctrl, ctrlRef, err := world_block_engine.StartEngineWithConfig(ctx, tb.Bus, conf)
	if err != nil {
		t.Fatal(err)
	}
	defer ctrlRef.Release()
	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Commit a write that changes nothing.
	tx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Commit objects and quads in one write.
	tx, err = eng.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	keys := []string{"game", "a", "b", "c"}
	for _, key := range keys {
		obj, err := tx.CreateObject(ctx, key, nil)
		world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, key := range keys[1:] {
		if err := tx.SetGraphQuad(ctx, world.NewGraphQuadWithKeys(key, "<type>", keys[0], "")); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
