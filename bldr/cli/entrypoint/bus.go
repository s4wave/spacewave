//go:build !js

package cli_entrypoint

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	entrypoint_fatal "github.com/s4wave/spacewave/bldr/entrypoint/fatal"
	entrypoint_state "github.com/s4wave/spacewave/bldr/entrypoint/state"
	"github.com/s4wave/spacewave/bldr/storage"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// CliBusImpl implements the CliBus interface for CLI binaries.
type CliBusImpl struct {
	// ctx is canceled when Release begins.
	ctx context.Context
	// b owns controller execution and cleanup.
	b bus.Bus
	// le reports lifecycle failures.
	le *logrus.Entry
	// sr provides the CLI controller factories.
	sr *static.Resolver
	// storageID identifies the CLI storage method.
	storageID string
	// worldEngineID identifies the CLI world engine.
	worldEngineID string
	// stateRoot is the state directory the bus opens its volumes in.
	stateRoot string
	// vol stores CLI state.
	vol volume.Volume
	// worldEngine manages the CLI world.
	worldEngine world.Engine
	// worldState exposes the CLI world state.
	worldState world.WorldState
	// rels runs bus cleanup before caller cleanup.
	rels []func()
	// releaseOnce joins concurrent Release calls.
	releaseOnce sync.Once
}

// BuildCliBus opens the shared project state for a native CLI daemon, with
// each Volume config the storage builds passed through configureVolume, nil
// for none. The caller must hold the state-root lease until Release finishes.
func BuildCliBus(rctx context.Context, le *logrus.Entry, projectID, stateRoot string, configureVolume storage.ConfigureVolumeFunc) (*CliBusImpl, error) {
	// Cancel bus work before releasing controller and storage references.
	ctx, ctxCancel := context.WithCancel(rctx)
	var rels []func()
	var b bus.Bus
	rel := func() {
		ctxCancel()
		if b != nil {
			if err := b.Close(); err != nil {
				le.WithError(err).Error("closing CLI bus")
			}
		}
		for _, fn := range rels {
			fn()
		}
	}

	// Create the controller bus before registering native storage factories.
	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		rel()
		return nil, err
	}

	// Add controller factories.
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(bucket_setup.NewFactory(b))
	sr.AddFactory(storage_volume.NewFactory(b))
	sr.AddFactory(block_store_bucket.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))

	// Add the configset controller.
	configSetCtrl, _ := configset_controller.NewController(le, b)
	relConfigSetCtrl, err := b.AddController(ctx, configSetCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relConfigSetCtrl)

	// Attach the default storage controller, its Volumes configured.
	storageID := default_storage.StorageID
	storageCtrl := default_storage.NewControllerWithStorage(
		storageID,
		storage.ConfigureAll(default_storage.BuildStorage(b, stateRoot), configureVolume),
	)
	relStorageCtrl, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relStorageCtrl)

	// Register the storage factories and require at least one storage method.
	if !storageCtrl.RegisterFactories(b, sr) {
		rel()
		return nil, errors.New("no available storage methods")
	}

	// Start the storage volume.
	volCtrl, volCtrlRef, err := storage_volume.ExecVolumeController(ctx, b, entrypoint_state.NewVolumeConfig(storageID))
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, volCtrlRef.Release)

	// Keep the actual mounted volume identity for all world operations.
	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		rel()
		return nil, err
	}

	// Start the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, nodeCtrlRef.Release)

	// Open the project World through the same schema as the desktop entrypoint.
	engConf, err := entrypoint_state.NewWorldConfig(projectID, vol.GetID())
	if err != nil {
		rel()
		return nil, err
	}
	engineID := engConf.GetEngineId()
	bucketConf, err := bucket.NewConfig(engConf.GetBucketId(), 1, nil)
	if err != nil {
		rel()
		return nil, err
	}
	if _, err := bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(bucketConf, vol.GetID())); err != nil {
		rel()
		return nil, err
	}

	// Retain the world engine until bus cleanup and publish a writable World
	// facade owned by this bus.
	eng, engRef, err := world.StartEngine(ctx, b, engConf, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, engRef.Release)
	worldState := world.NewEngineWorldState(eng, true)

	// Return the bus and the single cleanup boundary.
	return &CliBusImpl{
		ctx:           ctx,
		b:             b,
		le:            le,
		sr:            sr,
		storageID:     storageID,
		worldEngineID: engineID,
		stateRoot:     stateRoot,
		vol:           vol,
		worldEngine:   eng,
		worldState:    worldState,
		rels:          []func(){rel},
	}, nil
}

// GetBus returns the controller bus.
func (c *CliBusImpl) GetBus() bus.Bus {
	return c.b
}

// GetLogger returns the root logger.
func (c *CliBusImpl) GetLogger() *logrus.Entry {
	return c.le
}

// RegisterFactories binds factory builders to the CLI bus and resolver.
// Nil builders are skipped.
func (c *CliBusImpl) RegisterFactories(builders ...AddFactoryFunc) {
	for _, build := range builders {
		if build == nil {
			continue
		}
		for _, factory := range build(c.b) {
			c.sr.AddFactory(factory)
		}
	}
}

// applyConfigSets applies the merged config sets and returns their reference.
// It returns no reference when no sets are configured.
func (c *CliBusImpl) applyConfigSets(ctx context.Context, builders []BuildConfigSetFunc) (directive.Reference, error) {
	// Gather the config sets contributed by the linked packages.
	var merged []configset.ConfigSet
	for _, build := range builders {
		sets, err := build(ctx, c.b, c.le)
		if err != nil {
			return nil, err
		}
		merged = append(merged, sets...)
	}

	// Assert the merged set and leave its cleanup to the caller.
	if len(merged) == 0 {
		return nil, nil
	}
	set := configset.MergeConfigSets(merged...)
	_, ref, err := c.b.AddDirective(ctx, configset.NewApplyConfigSet(set), nil)
	return ref, err
}

// waitForServe logs startup and waits for shutdown or a fatal controller error.
func (c *CliBusImpl) waitForServe() error {
	// Report that the CLI bus is ready.
	c.le.Info("started, press ctrl+c to stop")

	// Stop when the bus ends or a controller reports that it cannot serve.
	select {
	case <-c.ctx.Done():
		return nil
	case <-entrypoint_fatal.Chan():
		return entrypoint_fatal.Err()
	}
}

// GetVolume returns the volume used for state.
func (c *CliBusImpl) GetVolume() volume.Volume {
	return c.vol
}

// GetStateRoot returns the state directory the bus opens its volumes in.
func (c *CliBusImpl) GetStateRoot() string {
	return c.stateRoot
}

// GetWorldEngineID returns the world engine ID.
func (c *CliBusImpl) GetWorldEngineID() string {
	return c.worldEngineID
}

// GetWorldEngine returns the world engine instance.
func (c *CliBusImpl) GetWorldEngine() world.Engine {
	return c.worldEngine
}

// GetWorldState returns the world state instance.
func (c *CliBusImpl) GetWorldState() world.WorldState {
	return c.worldState
}

// GetPluginHostObjectKey returns the plugin host object key.
func (c *CliBusImpl) GetPluginHostObjectKey() string {
	return ""
}

// AddRelease registers cleanup to run after the bus-owned resources.
// Callers register cleanup before Release begins.
func (c *CliBusImpl) AddRelease(release func()) {
	if release != nil {
		c.rels = append(c.rels, release)
	}
}

// Release cancels and joins the bus before running caller cleanup.
func (c *CliBusImpl) Release() {
	c.releaseOnce.Do(func() {
		for _, rel := range c.rels {
			rel()
		}
	})
}

// _ is a type assertion.
var _ CliBus = (*CliBusImpl)(nil)
