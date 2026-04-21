//go:build !js

package cli_entrypoint

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	block_store_bucket "github.com/s4wave/spacewave/db/block/store/bucket"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	lookup_concurrent "github.com/s4wave/spacewave/db/bucket/lookup/concurrent"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/sirupsen/logrus"
)

// cliTransformConf is the block transform conf to use.
var cliTransformConf = []config.Config{
	&transform_s2.Config{},
}

// CliBusImpl implements the CliBus interface for CLI binaries.
type CliBusImpl struct {
	ctx           context.Context
	b             bus.Bus
	le            *logrus.Entry
	sr            *static.Resolver
	storageID     string
	worldEngineID string
	vol           volume.Volume
	worldEngine   world.Engine
	worldState    world.WorldState
	rels          []func()
}

// _ is a type assertion
var _ CliBus = (*CliBusImpl)(nil)

// BuildCliBus builds a lightweight bus for CLI binaries.
func BuildCliBus(rctx context.Context, le *logrus.Entry, stateRoot string) (*CliBusImpl, error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	var rels []func()
	rel := func() {
		for _, fn := range rels {
			fn()
		}
		ctxCancel()
	}

	b, sr, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		rel()
		return nil, err
	}

	// add controller factories
	sr.AddFactory(node_controller.NewFactory(b))
	sr.AddFactory(lookup_concurrent.NewFactory(b))
	sr.AddFactory(bucket_setup.NewFactory(b))
	sr.AddFactory(storage_volume.NewFactory(b))
	sr.AddFactory(block_store_bucket.NewFactory(b))
	sr.AddFactory(world_block_engine.NewFactory(b))

	// add the configset controller
	configSetCtrl, _ := configset_controller.NewController(le, b)
	relConfigSetCtrl, err := b.AddController(ctx, configSetCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relConfigSetCtrl)

	// attach the default storage controller
	storageID := default_storage.StorageID
	storageCtrl := default_storage.NewController(storageID, b, stateRoot)
	relStorageCtrl, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relStorageCtrl)

	// ensure there is at least one storage method
	storageMethods := storageCtrl.GetStorage()
	if len(storageMethods) == 0 {
		rel()
		return nil, errors.New("no available storage methods")
	}

	// add the storage method factories
	for _, storageMethod := range storageMethods {
		storageMethod.AddFactories(b, sr)
	}

	// start the storage volume
	volCtrl, volCtrlRef, err := storage_volume.ExecVolumeController(ctx, b, &storage_volume.Config{
		StorageId:       storageID,
		StorageVolumeId: "cli",
		VolumeConfig: &volume_controller.Config{
			VolumeIdAlias: []string{bldr_plugin.PluginVolumeID},
		},
	})
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, volCtrlRef.Release)

	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		rel()
		return nil, err
	}

	// start the node controller
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, nodeCtrlRef.Release)

	// start the world engine
	engineBucketID := "bldr/cli"
	engineObjStoreID := engineBucketID
	engineID := "bldr/cli"

	bucketConf, err := bucket.NewConfig(engineBucketID, 1, nil, nil)
	if err != nil {
		rel()
		return nil, err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(bucketConf, vol.GetID()))
	if err != nil {
		rel()
		return nil, err
	}

	transformConf, err := block_transform.NewConfig(cliTransformConf)
	if err != nil {
		rel()
		return nil, err
	}
	initRef := &bucket.ObjectRef{
		BucketId:      engineBucketID,
		TransformConf: transformConf,
	}

	engConf := world_block_engine.NewConfig(
		engineID,
		vol.GetID(), engineBucketID,
		engineObjStoreID,
		initRef,
		nil,
		false,
	)

	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(ctx, b, engConf)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, worldCtrlRef.Release)

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		rel()
		return nil, err
	}
	worldState := world.NewEngineWorldState(eng, true)

	return &CliBusImpl{
		ctx:           ctx,
		b:             b,
		le:            le,
		sr:            sr,
		storageID:     storageID,
		worldEngineID: engineID,
		vol:           vol,
		worldEngine:   eng,
		worldState:    worldState,
		rels:          rels,
	}, nil
}

// GetContext returns the bus context.
func (c *CliBusImpl) GetContext() context.Context {
	return c.ctx
}

// GetBus returns the controller bus.
func (c *CliBusImpl) GetBus() bus.Bus {
	return c.b
}

// GetLogger returns the root logger.
func (c *CliBusImpl) GetLogger() *logrus.Entry {
	return c.le
}

// GetStaticResolver returns the static controller resolver.
func (c *CliBusImpl) GetStaticResolver() *static.Resolver {
	return c.sr
}

// GetVolume returns the volume used for state.
func (c *CliBusImpl) GetVolume() volume.Volume {
	return c.vol
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

// Release releases all resources held by the bus.
func (c *CliBusImpl) Release() {
	for _, rel := range c.rels {
		rel()
	}
}
