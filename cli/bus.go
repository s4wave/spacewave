package cli

import (
	"context"
	"errors"
	"os"
	"path"

	"github.com/aperturerobotics/bldr/core"
	"github.com/aperturerobotics/bldr/plugin"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	plugin_host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	"github.com/aperturerobotics/bldr/storage"
	default_storage "github.com/aperturerobotics/bldr/storage/default"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	"github.com/aperturerobotics/hydra/bucket"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/sirupsen/logrus"
)

// devtoolTransformConf is the block transform conf to use.
var devtoolTransformConf = []config.Config{
	&transform_s2.Config{},
}

// DevtoolBus contains a built devtool bus.
type DevtoolBus struct {
	// ctx contains the context
	ctx context.Context
	// b contains the bus
	b bus.Bus
	// le contains the root logger
	le *logrus.Entry
	// sr contains the static resolver
	sr *static.Resolver
	// worldEngineID is the world engine id for state
	worldEngineID string
	// engineBucketID is the bucket used for world engine state storage
	engineBucketID string
	// engineObjectStoreID is the bucket used for root world engine state ref
	engineObjectStoreID string
	// pluginHostObjectKey is the object key used for the PluginHost
	pluginHostObjectKey string
	// pluginHostCtrl is the plugin host controller
	pluginHostCtrl *plugin_host_controller.Controller
	// st contains the storage method
	st storage.Storage
	// stConf is the storage config
	stConf config.Config
	// vol is the volume used for state
	vol volume.Volume
	// worldEngine is the world engine instance.
	worldEngine world.Engine
	// worldState is the world state instance.
	worldState world.WorldState
	// rels are the release funcs
	rels []func()
}

// BuildDevtoolBus builds the storage and bus for the devtool.
// Returns a set of functions to call to release the controllers.
func BuildDevtoolBus(rctx context.Context, le *logrus.Entry, stateRoot string) (*DevtoolBus, error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	sr.AddFactory(world_block_engine.NewFactory(b))
	sr.AddFactory(plugin_host_process.NewFactory(b))

	// build the plugin state paths
	pluginHostObjectKey := "devtool/plugin-host"
	pluginsRoot := path.Join(stateRoot, "p")
	pluginsDistRoot := path.Join(pluginsRoot, "d")
	if err := os.MkdirAll(pluginsDistRoot, 0755); err != nil {
		ctxCancel()
		return nil, err
	}
	pluginsStateRoot := path.Join(pluginsRoot, "s")
	if err := os.MkdirAll(pluginsStateRoot, 0755); err != nil {
		ctxCancel()
		return nil, err
	}

	storageMethods := default_storage.BuildStorage(b, stateRoot)
	if len(storageMethods) == 0 {
		ctxCancel()
		return nil, errors.New("no available storage methods")
	}

	storageMethod := storageMethods[0]
	storageMethod.AddFactories(b, sr)
	stConf := storageMethod.BuildVolumeConfig("bldr")

	volCtrli, _, diRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(stConf),
		ctxCancel,
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	volCtrl, ok := volCtrli.(volume.Controller)
	if !ok {
		ctxCancel()
		return nil, errors.New("volume controller returned invalid value")
	}

	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// start the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, false, nil)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// start devtool world
	engineBucketID := "bldr/devtool"
	engineObjStoreID := engineBucketID
	engineID := "bldr"

	// create bucket if it doesn't exist
	bucketConf, err := bucket.NewConfig(engineBucketID, 1, nil, nil)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(bucketConf, vol.GetID()))
	if err != nil {
		ctxCancel()
		return nil, err
	}

	transformConf, err := block_transform.NewConfig(devtoolTransformConf)
	if err != nil {
		ctxCancel()
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
	)
	engConf.Verbose = false
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		b,
		engConf,
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	worldState := world.NewEngineWorldState(ctx, eng, true)

	// register the world operation types for plugin host
	go b.ExecuteController(ctx, world.NewLookupOpController("bldr-plugin-host-ops", engineID, plugin_host.LookupOp))

	// build the plugin host controller
	pluginHostCtrlObj, _, pluginHostRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(host_process.NewConfig(
			engineID,
			pluginHostObjectKey,
			vol.GetPeerID(),
			pluginsStateRoot,
			pluginsDistRoot,
		)),
		ctxCancel,
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	pluginHostCtrl := pluginHostCtrlObj.(*plugin_host_controller.Controller)

	// TODO: load the root plugin ??
	// TODO: move to directive
	go func() {
		_ = pluginHostCtrl.RunPlugin(ctx, "root-plugin", func(ps *plugin.PluginStatus) error {
			le.Infof("root-plugin: status changed: %s", ps.String())
			return nil
		})
	}()

	return &DevtoolBus{
		ctx:                 ctx,
		b:                   b,
		le:                  le,
		sr:                  sr,
		worldEngineID:       engineID,
		engineBucketID:      engineBucketID,
		engineObjectStoreID: engineObjStoreID,
		pluginHostObjectKey: pluginHostObjectKey,
		pluginHostCtrl:      pluginHostCtrl,
		st:                  storageMethod,
		stConf:              stConf,
		vol:                 vol,
		worldEngine:         eng,
		worldState:          worldState,
		rels: []func(){
			pluginHostRef.Release,
			worldCtrlRef.Release,
			nodeCtrlRef.Release,
			ctxCancel,
			diRef.Release,
			func() { volCtrl.Close() },
		},
	}, nil
}

// GetContext returns the context.
func (d *DevtoolBus) GetContext() context.Context {
	return d.ctx
}

// GetBus returns the bus.
func (d *DevtoolBus) GetBus() bus.Bus {
	return d.b
}

// GetLogger returns the root logger
func (d *DevtoolBus) GetLogger() *logrus.Entry {
	return d.le
}

// GetStaticResolver returns the static controller resolver.
func (d *DevtoolBus) GetStaticResolver() *static.Resolver {
	return d.sr
}

// GetStorage returns the storage.
func (d *DevtoolBus) GetStorage() storage.Storage {
	return d.st
}

// GetStorageConf returns the storage config.
func (d *DevtoolBus) GetStorageConf() config.Config {
	return d.stConf
}

// GetVolume returns the storage volume in use.
func (d *DevtoolBus) GetVolume() volume.Volume {
	return d.vol
}

// GetWorldEngine returns the world engine instance.
func (d *DevtoolBus) GetWorldEngine() world.Engine {
	return d.worldEngine
}

// GetWorldState returns the world state handle.
func (d *DevtoolBus) GetWorldState() world.WorldState {
	return d.worldState
}

// Release releases the devtool bus.
func (d *DevtoolBus) Release() {
	for _, rel := range d.rels {
		rel()
	}
}
