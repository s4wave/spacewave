package volume_world_test

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	b58 "github.com/mr-tron/base58/base58"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/volume"
	volume_test "github.com/s4wave/spacewave/db/volume/test"
	volume_world "github.com/s4wave/spacewave/db/volume/world"
	"github.com/s4wave/spacewave/db/world/testbed"
	"github.com/zeebo/blake3"
)

// TestWorldVolume tests the world backed volume.
func TestWorldVolume(t *testing.T) {
	// Give the World-backed volume test a context for its operations.
	ctx := context.Background()

	// Start a World testbed with the World-backed volume controller factory.
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(volume_world.NewFactory(tb.Bus))

	// Select the testbed volume, bucket, and engine for the volume configuration.
	le := tb.Logger
	vol := tb.Volume
	volumeID := vol.GetID()
	objectStoreID := "test-block-volume-store"
	bucketID := tb.EngineBucketID
	engineID := tb.EngineID

	// Derive a test encryption key for the volume block store.
	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/volume/block/test: block_test.go", []byte(objectStoreID), encKey)
	le.Infof("using encryption key: %s", b58.Encode(encKey))

	// Configure the block encryption transform for the World-backed volume.
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// initHeadRef is only used if the volume has not been previously inited.
	initHeadRef := &bucket.ObjectRef{
		BucketId:      bucketID,
		TransformConf: transformConf,
	}

	// start the volume
	objKey := "test-volume"
	vctrl, _, diRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(volume_world.NewConfig(
			volumeID,
			bucketID,
			engineID,
			objKey,
			initHeadRef,
		)),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer diRef.Release()

	// Acquire the volume exposed by the running World-backed controller.
	volCtrl := vctrl.(volume.Controller)
	bvol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check volume behavior
	if err := volume_test.CheckVolume(ctx, bvol); err != nil {
		t.Fatal(err.Error())
	}
}
