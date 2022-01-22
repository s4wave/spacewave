package volume_block_e2e

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/util/blockenc"
	"github.com/aperturerobotics/hydra/volume"
	volume_test "github.com/aperturerobotics/hydra/volume/test"
	volume_world "github.com/aperturerobotics/hydra/volume/world"
	"github.com/aperturerobotics/hydra/world/testbed"
	b58 "github.com/mr-tron/base58/base58"
	"github.com/zeebo/blake3"
)

// TestBlockVolume tests the block graph backed volume.
func TestBlockVolume(t *testing.T) {
	ctx := context.Background()

	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(volume_world.NewFactory(tb.Bus))

	le := tb.Logger
	vol := tb.Volume
	volumeID := vol.GetID()
	objectStoreID := "test-block-volume-store"
	bucketID := tb.EngineBucketID
	engineID := tb.EngineID

	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/volume/block/test: block_test.go", []byte(objectStoreID), encKey)
	le.Infof("using encryption key: %s", b58.Encode(encKey))

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

	volCtrl := vctrl.(volume.Controller)
	bvol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}

	// check volume behavior
	if err := volume_test.CheckVolume(ctx, le, bvol); err != nil {
		t.Fatal(err.Error())
	}
}
