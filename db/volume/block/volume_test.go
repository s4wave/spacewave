package volume_block_test

import (
	"context"
	"crypto/rand"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	b58 "github.com/mr-tron/base58/base58"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/volume"
	volume_block "github.com/s4wave/spacewave/db/volume/block"
	volume_test "github.com/s4wave/spacewave/db/volume/test"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// TestBlockVolume tests the block graph backed volume.
func TestBlockVolume(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(volume_block.NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	objectStoreID := "test-block-volume-store"
	bucketID := tb.BucketId

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

	// note: use the same transform config to transform the HEAD ref
	// (usually we would use a separate one)
	stateTransformConf := transformConf

	// init the volume with the key
	nvolPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err.Error())
	}

	sfs := transform_all.BuildFactorySet()

	bcs, err := bucket_lookup.BuildCursor(ctx, tb.Bus, le, sfs, volumeID, initHeadRef, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	volBlockConf := &volume_block.Config{NoGenerateKey: true}
	initHeadRef, err = volume_block.InitVolume(ctx, le, volBlockConf, bcs, nvolPriv)
	if err != nil {
		t.Fatal(err.Error())
	}

	// start the volume
	volCtrl, _, diRef, err := loader.WaitExecControllerRunningTyped[volume.Controller](
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(volume_block.NewConfig(
			volumeID,
			bucketID,
			objectStoreID,
			initHeadRef,
			stateTransformConf,
		)),
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

	// check volume key
	t.Log(bvol.GetPeerID().String())
	bvolPeer, err := bvol.GetPeer(ctx, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	bvolPriv, err := bvolPeer.GetPrivKey(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !bvolPriv.GetPublic().Equals(nvolPriv.GetPublic()) {
		t.Fatal("key mismatch")
	}
	if tp := bvolPriv.Type(); tp != crypto.KeyType_Ed25519 {
		t.Fatalf("expected ed25519 but got %s", tp.String())
	}
}
