package testbed

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/pkg/errors"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	transform_chksum "github.com/s4wave/spacewave/db/block/transform/chksum"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
	"github.com/zeebo/blake3"
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

	var worldVerbose bool
	for _, opt := range opts {
		switch o := opt.(type) {
		case *withWorldVerbose:
			worldVerbose = o.verbose
		default:
			return nil, errors.Errorf("unrecognized testbed option: %#v", o)
		}
	}

	t = &Testbed{Testbed: tb}
	ctx, b, sr := tb.Context, tb.Bus, tb.StaticResolver

	core.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	sr.AddFactory(world_block_engine.NewFactory(tb.Bus))

	// Construct the world engine.
	t.EngineID = "testbed-engine"
	t.EngineVolumeID = tb.Volume.GetID()
	t.EngineBucketID = tb.BucketId
	t.EngineObjectStoreID = t.EngineID + "-store"

	// note: do not use this crypto key for anything else
	key := make([]byte, 32)
	blake3.DeriveKey("hydra/world/testbed "+t.EngineBucketID, []byte("testbed"), key)

	// create a initial ref with a encryption config
	transformConf, err := block_transform.NewConfig([]config.Config{
		&transform_chksum.Config{},
		&transform_s2.Config{},
		&transform_blockenc.Config{
			BlockEnc: blockenc.BlockEnc_BlockEnc_XCHACHA20_POLY1305,
			Key:      key,
		},
	})
	if err != nil {
		return nil, err
	}
	initRef := &bucket.ObjectRef{
		BucketId:      t.EngineBucketID,
		TransformConf: transformConf,
	}
	engConf := world_block_engine.NewConfig(
		t.EngineID,
		t.EngineVolumeID, t.EngineBucketID,
		t.EngineObjectStoreID,
		initRef,
		nil,
		false,
	)
	engConf.Verbose = worldVerbose
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		b,
		engConf,
	)
	if err != nil {
		return nil, err
	}
	rels = append(rels, worldCtrlRef.Release)
	t.EngineController = worldCtrl

	engh, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		return nil, err
	}
	t.Engine = engh
	t.BusEngine = world.NewBusEngine(ctx, b, t.EngineID)
	t.WorldState = world.NewEngineWorldState(t.BusEngine, true)
	return t, nil
}

// Default constructs the default testbed arrangement.
func Default(ctx context.Context, opts ...Option) (*Testbed, error) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return nil, err
	}
	tb2, err := NewTestbed(tb, opts...)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}

// WithTestbedOptions constructs the testbed with the given testbed options.
func WithTestbedOptions(ctx context.Context, testbedOptions []testbed.Option, worldOpts []Option) (*Testbed, error) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	tb, err := testbed.NewTestbed(ctx, le, testbedOptions...)
	if err != nil {
		return nil, err
	}
	tb2, err := NewTestbed(tb, worldOpts...)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}
