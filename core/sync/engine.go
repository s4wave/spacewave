// Package sync hosts a standalone World engine over caller-selected storage.
package sync

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	"github.com/s4wave/spacewave/db/bucket"
	db_storage "github.com/s4wave/spacewave/db/core/storage"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// Engine owns one ready World and the bus that runs its storage controllers.
// Callers stop admitting operations before Close and must not use it afterwards.
type Engine struct {
	// Bus owns all controllers, including their joined shutdown.
	Bus bus.Bus
	// World is the ready engine capability.
	World world.Engine
	// State provides convenience operations over World transactions.
	State world.WorldState
	// cancel stops the owned controller context after durability is fenced.
	cancel context.CancelFunc
	// refs retain the controller directives until the bus has stopped.
	refs []directive.Reference
	// closeOnce joins concurrent Close calls.
	closeOnce sync.Once
	// closeErr is the shared result of shutdown.
	closeErr error
}

// Open constructs a ready World without starting accounts, networking, or apps.
// Storage owns its dataset; closing the engine releases handles and never deletes it.
func Open(ctx context.Context, le *logrus.Entry, backing storage.Storage) (_ *Engine, resultErr error) {
	// Validate the arguments and prepare the engine with a cancelable context.
	if backing == nil {
		return nil, errors.New("sync engine requires storage")
	}
	if le == nil {
		le = logrus.NewEntry(logrus.New())
	}

	// Create the engine with a cancelable context and failure cleanup.
	ctx, cancel := context.WithCancel(ctx)
	engine := &Engine{cancel: cancel}
	defer func() {
		if resultErr != nil {
			resultErr = errors.Join(resultErr, engine.Close())
		}
	}()

	// Start a core bus and register the storage and World factories.
	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		return nil, err
	}
	engine.Bus = b
	db_storage.AddFactories(b, sr)
	backing.AddFactories(b, sr)
	sr.AddFactory(storage_volume.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))

	// Register the KV controller factory.
	if err := registerKV(ctx, b); err != nil {
		return nil, err
	}

	// Keep the supplied storage discoverable for the volume controller's lifetime.
	storageController := storage_controller.BuildStorageController(
		"sync", []storage.Storage{backing},
		controller.NewInfo("sync/storage", controller.MustParseVersion("0.0.1"), "sync engine storage"),
	)
	if _, err := b.AddController(ctx, storageController, nil); err != nil {
		return nil, err
	}

	// Start the configset and node controllers and retain their references.
	_, _, configRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&configset_controller.Config{}), nil)
	if err != nil {
		return nil, err
	}
	engine.refs = append(engine.refs, configRef)
	_, _, nodeRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&node_controller.Config{}), nil)
	if err != nil {
		return nil, err
	}
	engine.refs = append(engine.refs, nodeRef)

	// Start the volume controller for the sync storage.
	volumeController, volumeRef, err := storage_volume.ExecVolumeController(ctx, b, &storage_volume.Config{
		StorageId: "sync", StorageVolumeId: "sync",
	})
	if err != nil {
		return nil, err
	}
	engine.refs = append(engine.refs, volumeRef)

	// Configure the sync bucket and start the World engine.
	volume, err := volumeController.GetVolume(ctx)
	if err != nil {
		return nil, err
	}
	if _, _, _, err := volume.ApplyBucketConfig(ctx, &bucket.Config{Id: "sync", Rev: 1}); err != nil {
		return nil, err
	}
	worldController, worldRef, err := world_block_engine.StartEngineWithConfig(ctx, b, world_block_engine.NewConfig(
		"sync", volume.GetID(), "sync", "sync", &bucket.ObjectRef{BucketId: "sync"}, nil, false,
	))
	if err != nil {
		return nil, err
	}
	engine.refs = append(engine.refs, worldRef)

	// Wait for the World engine and expose its writable state.
	engine.World, err = worldController.GetWorldEngine(ctx)
	if err != nil {
		return nil, err
	}
	engine.State = world.NewEngineWorldState(engine.World, true)
	return engine, nil
}

// Close fences acknowledged data and joins all owned controllers.
// Failure still releases every handle and remains observable on subsequent calls.
func (e *Engine) Close() error {
	e.closeOnce.Do(func() {
		// Fence acknowledged data before tearing down the bus.
		if e.State != nil {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_, e.closeErr = e.State.Sync(ctx)
			cancel()
		}
		e.cancel()
		if e.Bus != nil {
			e.closeErr = errors.Join(e.closeErr, e.Bus.Close())
		}
		if e.Bus != nil {
			for _, instance := range e.Bus.GetDirectives() {
				instance.Close()
			}
		}

		// Release the retained controller references.
		for _, ref := range e.refs {
			ref.Release()
		}
		e.refs = nil
	})
	return e.closeErr
}
