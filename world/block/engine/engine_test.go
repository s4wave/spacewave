package world_block_engine

import (
	"context"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/hydra/testbed"
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
	tb.StaticResolver.AddFactory(NewFactory(tb.Bus))

	vol := tb.Volume
	volumeID := vol.GetID()
	engineID := "test-world-engine"
	objectStoreID := "test-world-engine-store"

	/*
		bktCs, err := tb.BuildEmptyCursor(ctx)
		if err != nil {
			t.Fatal(err.Error())
		}
	*/

	conf := &Config{
		BucketId:      testbed.BucketId,
		EngineId:      engineID,
		VolumeId:      volumeID,
		ObjectStoreId: objectStoreID,
		// InitHeadRef: *bucket.ObjectRef,
	}
	ctrli, ctrlInst, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		tb.Bus,
		resolver.NewLoadControllerWithConfig(conf),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer ctrlRef.Release()
	_ = ctrlInst

	ctrl := ctrli.(*Controller)

	eng, err := ctrl.GetWorldEngine(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = world_mock.TestWorldEngine(ctx, eng)
	if err != nil {
		t.Fatal(err.Error())
	}
}
