package unixfs_world_e2e

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/config"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_blockenc "github.com/aperturerobotics/hydra/block/transform/blockenc"
	transform_chksum "github.com/aperturerobotics/hydra/block/transform/chksum"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	"github.com/aperturerobotics/hydra/bucket"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_billy "github.com/aperturerobotics/hydra/unixfs/billy"
	unixfs_block "github.com/aperturerobotics/hydra/unixfs/block"
	unixfs_world "github.com/aperturerobotics/hydra/unixfs/world"
	"github.com/aperturerobotics/hydra/util/blockenc"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	billy_util "github.com/go-git/go-billy/v5/util"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
)

// TestInitUnixFSDemo tests the InitUnixFSDemoOp operation.
func TestInitUnixFSDemo(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	tb.StaticResolver.AddFactory(world_block_engine.NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	engineID := "test-world-engine"
	objectStoreID := "test-world-engine-store"
	bucketID := tb.BucketId

	encKey := make([]byte, 32)
	blake3.DeriveKey("hydra/test/unixfs: init_unixfs_demo_test.go", []byte(objectStoreID), encKey)

	xfrmConf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      encKey,
		},
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// initWorldRef is only used if the world has not been previously inited.
	initWorldRef := &bucket.ObjectRef{
		BucketId:      bucketID,
		TransformConf: xfrmConf,
	}

	// initialize world engine
	_, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			initWorldRef,
			xfrmConf,
			false,
		),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer worldCtrlRef.Release()

	// provide op handlers to bus
	opc := world.NewLookupOpController("test-unixfs-demo-ops", engineID, LookupInitUnixFSDemoOp)
	_, err = tb.Bus.AddController(ctx, opc, nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// uses directive to look up the engine
	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	// uses short-lived engine txs to implement world state
	ws := world.NewEngineWorldState(busEngine, true)

	sender := tb.Volume.GetPeerID()
	objKey := "test-unixfs-demo"
	ts := unixfs_block.FillPlaceholderTimestamp(nil).AsTime()

	// apply the InitUnixFSDemoOp operation
	_, sysErr, err := InitUnixFSDemo(ctx, ws, sender, objKey, ts)
	if err != nil {
		if sysErr {
			t.Fatalf("system error: %v", err)
		}
		t.Fatal(err.Error())
	}

	// verify the filesystem was created with expected structure
	rootFSCursor, err := unixfs_world.FollowUnixfsRef(ctx, le, ws, &unixfs_world.UnixfsRef{ObjectKey: objKey}, sender, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	fsh, err := unixfs.NewFSHandle(rootFSCursor)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsh.Release()

	// wrap in billy filesystem for testing
	fs := unixfs_billy.NewBillyFilesystem(ctx, fsh, "", ts)

	// verify hello.txt exists and has correct content
	helloContent, err := billy_util.ReadFile(fs, "hello.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	expectedContent := "Hello world from Go!\n"
	if string(helloContent) != expectedContent {
		t.Errorf("hello.txt content mismatch: got %q, want %q", string(helloContent), expectedContent)
	}

	// verify world.md exists
	_, err = fs.Stat("world.md")
	if err != nil {
		t.Fatal(err.Error())
	}

	// verify /test/dir directory structure exists
	_, err = fs.Stat("test/dir")
	if err != nil {
		t.Fatal(err.Error())
	}

	le.Info("verified UnixFS demo filesystem structure")
}
