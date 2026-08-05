package testbed

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
	boilerplate_controller "github.com/aperturerobotics/controllerbus/example/boilerplate/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	storage_inmem "github.com/s4wave/spacewave/bldr/storage/inmem"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
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

	// EngineBucketID identifies the bucket the engine is attached to.
	EngineBucketID string
	// EngineVolumeID identifies the volume the engine uses for state.
	EngineVolumeID string
	// EngineObjectStoreID identifies the object store the engine uses for state.
	EngineObjectStoreID string
	// EngineID identifies the engine on the bus.
	EngineID string
	// Engine is a direct reference to the running world engine.
	Engine world.Engine
	// EngineController contains the world engine controller.
	EngineController *world_block_engine.Controller
	// BusEngine locates the engine through bus directives.
	BusEngine world.Engine
	// WorldState exposes the BusEngine-backed engine state.
	WorldState world.WorldState
	// StorageID identifies the storage controller.
	StorageID string
}

// NewTestbed constructs a new world testbed from a Hydra testbed.
func NewTestbed(tb *testbed.Testbed, opts ...Option) (t *Testbed, tbErr error) {
	// Reject a missing base testbed before allocating resources.
	if tb == nil {
		return nil, errors.New("testbed cannot be nil")
	}

	// Track cleanup callbacks for resources acquired below.
	var rels []func()
	defer func() {
		if tbErr != nil {
			for _, r := range rels {
				r()
			}
		}
	}()

	// Parse options controlling world verbosity and storage backends.
	var worldVerbose bool
	var storages []storage.Storage
	for _, opt := range opts {
		switch o := opt.(type) {
		case *withWorldVerbose:
			worldVerbose = o.verbose
		case *withStorages:
			storages = append([]storage.Storage(nil), o.storages...)
		default:
			return nil, errors.Errorf("unrecognized testbed option: %#v", o)
		}
	}

	// Initialize the extended testbed and its shared controller handles.
	t = &Testbed{Testbed: tb}
	ctx, b, sr := tb.Context, tb.Bus, tb.StaticResolver

	// Register core, engine, and storage controller factories.
	core.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	sr.AddFactory(world_block_engine.NewFactory(tb.Bus))
	sr.AddFactory(storage_volume.NewFactory(tb.Bus))

	// Assign stable identifiers for the world engine resources.
	t.EngineID = "testbed-engine"
	t.EngineVolumeID = tb.Volume.GetID()
	t.EngineBucketID = tb.BucketId
	t.EngineObjectStoreID = t.EngineID + "-store"

	// Build the engine transform configuration.
	transformConf, err := newEngineTransformConfig(t.EngineBucketID)
	if err != nil {
		return nil, err
	}

	// Prepare the initial bucket reference for engine startup.
	initRef := &bucket.ObjectRef{
		BucketId:      t.EngineBucketID,
		TransformConf: transformConf,
	}

	// Build the world engine configuration from the testbed identifiers.
	engConf := world_block_engine.NewConfig(
		t.EngineID,
		t.EngineVolumeID, t.EngineBucketID,
		t.EngineObjectStoreID,
		initRef,
		nil,
		false,
	)
	engConf.Verbose = worldVerbose

	// Start the world engine and retain its release callback.
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

	// Resolve the running engine and expose its bus-backed state.
	engh, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		return nil, err
	}
	t.Engine = engh
	t.BusEngine = world.NewBusEngine(ctx, b, t.EngineID)
	t.WorldState = world.NewEngineWorldState(t.BusEngine, true)

	// Select the storage controller identifier.
	storageID := "default"
	t.StorageID = storageID

	// Supply the default in-memory storage when no backend was requested.
	if len(storages) == 0 {
		storages = []storage.Storage{storage_inmem.NewInmemStorage()}
	}

	// Register each configured storage backend and reject nil entries.
	for _, st := range storages {
		if st == nil {
			return nil, errors.New("storage backend cannot be nil")
		}
		st.AddFactories(b, sr)
	}

	// Build and attach the storage controller.
	storageCtrl := storage_controller.BuildStorageController(
		storageID,
		storages,
		controller.NewInfo("storage/testbed", controller.MustParseVersion("0.0.1"), "testbed storage"),
	)
	storageCtrlRel, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		return nil, err
	}
	rels = append(rels, storageCtrlRel)

	return t, nil
}

// Default constructs the default testbed arrangement.
func Default(ctx context.Context, opts ...Option) (*Testbed, error) {
	// Initialize the default testbed logger.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Construct the base controller-bus testbed.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		return nil, err
	}

	// Extend the base testbed with world resources.
	tb2, err := NewTestbed(tb, opts...)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}

// WithTestbedOptions constructs the testbed with the given testbed options.
func WithTestbedOptions(ctx context.Context, testbedOptions []testbed.Option, worldOpts []Option) (*Testbed, error) {
	// Initialize the configured testbed logger.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Construct the base testbed with caller options.
	tb, err := testbed.NewTestbed(ctx, le, testbedOptions...)
	if err != nil {
		return nil, err
	}

	// Extend the base testbed with world options.
	tb2, err := NewTestbed(tb, worldOpts...)
	if err != nil {
		tb.Release()
		return nil, err
	}
	return tb2, nil
}
