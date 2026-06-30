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

	t = &Testbed{Testbed: tb}
	ctx, b, sr := tb.Context, tb.Bus, tb.StaticResolver

	core.AddFactories(b, sr)
	sr.AddFactory(boilerplate_controller.NewFactory(tb.Bus))
	sr.AddFactory(world_block_engine.NewFactory(tb.Bus))
	sr.AddFactory(storage_volume.NewFactory(tb.Bus))

	t.EngineID = "testbed-engine"
	t.EngineVolumeID = tb.Volume.GetID()
	t.EngineBucketID = tb.BucketId
	t.EngineObjectStoreID = t.EngineID + "-store"

	transformConf, err := newEngineTransformConfig(t.EngineBucketID)
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

	storageID := "default"
	t.StorageID = storageID
	if len(storages) == 0 {
		storages = []storage.Storage{storage_inmem.NewInmemStorage()}
	}
	for _, st := range storages {
		if st == nil {
			return nil, errors.New("storage backend cannot be nil")
		}
		st.AddFactories(b, sr)
	}
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
