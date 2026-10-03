package testbed

import (
	"context"
	"testing"

	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/core"
	"github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
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
	// Require the Hydra testbed that supplies World storage and controllers.
	if tb == nil {
		return nil, errors.New("testbed cannot be nil")
	}

	// Release acquired World directives if construction fails.
	var rels []func()
	defer func() {
		if tbErr != nil {
			for _, r := range rels {
				r()
			}
		}
	}()

	// Apply the requested World engine logging options.
	var worldVerbose bool
	for _, opt := range opts {
		switch o := opt.(type) {
		case *withWorldVerbose:
			worldVerbose = o.verbose
		default:
			return nil, errors.Errorf("unrecognized testbed option: %#v", o)
		}
	}

	// Attach the World testbed to the Hydra context and bus.
	t = &Testbed{Testbed: tb}
	ctx, b, sr := tb.Context, tb.Bus, tb.StaticResolver

	// Register the storage and World engine factories on the testbed bus.
	core.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	sr.AddFactory(world_block_engine.NewFactory(tb.Bus))

	// Construct the world engine.
	t.EngineID = "testbed-engine"
	t.EngineVolumeID = tb.Volume.GetID()
	t.EngineBucketID = tb.BucketId
	t.EngineObjectStoreID = t.EngineID + "-store"

	// Configure the World engine with its initial bucket object and transform.
	transformConf, err := NewEngineTransformConfig(t.EngineBucketID)
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

	// Start the World engine and retain its directive for failure cleanup.
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

	// Expose the running World engine through direct and bus-backed access.
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
	// Configure debug logging for the Hydra testbed.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Construct the Hydra storage testbed for the World engine.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return nil, err
	}

	// Attach the World engine and release Hydra storage if attachment fails.
	tb2, err := NewTestbed(tb, opts...)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}

// MustDefault constructs the default testbed arrangement, failing t if
// construction fails and releasing the testbed when the test finishes.
func MustDefault(t testing.TB, ctx context.Context, opts ...Option) *Testbed {
	// Construct the World testbed and register release with the test lifetime.
	t.Helper()
	tb, err := Default(ctx, opts...)
	if err != nil {
		t.Fatal(err)
		return nil
	}
	t.Cleanup(tb.Release)
	return tb
}

// WithTestbedOptions constructs the testbed with the given testbed options.
func WithTestbedOptions(ctx context.Context, testbedOptions []testbed.Option, worldOpts []Option) (*Testbed, error) {
	// Configure debug logging for the Hydra testbed.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Construct the Hydra storage testbed for the World engine.
	tb, err := testbed.NewTestbed(ctx, le, testbedOptions...)
	if err != nil {
		return nil, err
	}

	// Attach the World engine and release Hydra storage if attachment fails.
	tb2, err := NewTestbed(tb, worldOpts...)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}
