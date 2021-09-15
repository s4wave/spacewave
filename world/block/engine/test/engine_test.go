package world_block_engine_testing

import (
	"context"
	"testing"
	"time"

	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_block "github.com/aperturerobotics/hydra/world/block"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	world_mock "github.com/aperturerobotics/hydra/world/mock"
	"github.com/sirupsen/logrus"
)

// TestWorldEngineController tests constructing the engine controller, looking up
// the engine on the bus, & running some basic queries.
func TestWorldEngineController(t *testing.T) {
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
	bucketID := testbed.BucketId

	/*
		bktCs, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
	*/

	// initialize world engine
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			nil,
		),
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer worldCtrlRef.Release()

	// provide object op handlers to bus
	opc := world.NewLookupOpController("test-world-engine-ops", engineID, world_mock.LookupMockOp)
	go tb.Bus.ExecuteController(ctx, opc)

	// hack: wait for it to start
	<-time.After(time.Millisecond * 100)

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx, err := eng.NewTransaction(true)
	if err != nil {
		t.Fatal(err.Error())
	}
	engTx.Discard()

	// uses directive to look up the engine
	busEngine := world.NewBusEngine(ctx, tb.Bus, engineID)
	err = world_mock.TestWorldEngine(ctx, le, busEngine)
	if err != nil {
		t.Fatal(err.Error())
	}
	le.Info("world engine test suite passed")

	err = eng.AccessWorldState(ctx, nil, func(bls *bucket_lookup.Cursor) error {
		_, bcs := bls.BuildTransaction(nil)
		wi, err := bcs.Unmarshal(world_block.NewWorldBlock)
		if err != nil {
			t.Fatal(err.Error())
		}
		worldState := wi.(*world_block.World)
		le.Infof("world state after test suite: %s", worldState.String())
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// success
}
