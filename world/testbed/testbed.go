package testbed

import (
	"context"
	"errors"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/aperturerobotics/hydra/core"
	"github.com/aperturerobotics/hydra/testbed"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/sirupsen/logrus"
)

// Testbed is a constructed testbed.
type Testbed struct {
	*testbed.Testbed

	// EngineBucketID is the bucket the engine is attached to.
	EngineBucketID string
	// EngineVolumeID is the volume the engine uses for state.
	EngineVolumeID string
	// EngineObjectStoreID is the object store the engine uses for state.
	EngineObjectStoreID string
	// EngineID is the engine identifier on the bus.
	EngineID string
	// Engine contains a reference to the running world engine.
	// Queries the engine directly.
	Engine world.Engine
	// EngineController contains the world engine controller
	EngineController *world_block_engine.Controller
	// BusEngine uses directives to locate the Engine.
	BusEngine world.Engine
	// WorldState contains the BusEngine-backed Engine state.
	WorldState world.WorldState
}

// NewTestbed constructs a new world testbed from a Hydra testbed.
func NewTestbed(tb *testbed.Testbed, opts ...Option) (t *Testbed, tbErr error) {
	if tb == nil {
		return nil, errors.New("testbed cannot be nil")
	}

	var rels []func()
	defer func() {
		if tbErr != nil {
			for _, r := range rels {
				r()
			}
		}
	}()

	t = &Testbed{Testbed: tb}
	ctx, b, sr := tb.Context, tb.Bus, tb.StaticResolver

	core.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	sr.AddFactory(world_block_engine.NewFactory(tb.Bus))

	// Construct the world engine.
	t.EngineID = "testbed-engine"
	t.EngineVolumeID = tb.Volume.GetID()
	t.EngineBucketID = testbed.BucketId
	t.EngineObjectStoreID = t.EngineID + "-store"

	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		b,
		world_block_engine.NewConfig(
			t.EngineID,
			t.EngineVolumeID, t.EngineBucketID,
			t.EngineObjectStoreID,
			nil,
		),
	)
	if err != nil {
		return nil, err
	}
	rels = append(rels, worldCtrlRef.Release)
	t.EngineController = worldCtrl

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		return nil, err
	}
	t.Engine = eng
	t.BusEngine = world.NewBusEngine(ctx, b, t.EngineID)
	t.WorldState = world.NewEngineWorldState(ctx, t.BusEngine, true)
	return t, nil
}

// Default constructs the default testbed arrangement.
func Default(ctx context.Context) (*Testbed, error) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return nil, err
	}
	tb2, err := NewTestbed(tb)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}
