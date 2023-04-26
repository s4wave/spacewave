package dist_entrypoint

import (
	"context"
	"os"
	"path"

	"github.com/aperturerobotics/bifrost/peer"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	"github.com/aperturerobotics/bldr/storage"
	default_storage "github.com/aperturerobotics/bldr/storage/default"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	"github.com/aperturerobotics/hydra/volume"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// DistBus contains the distribution host bus.
type DistBus struct {
	// ctx contains the context
	ctx context.Context
	// b contains the bus
	b bus.Bus
	// le contains the root logger
	le *logrus.Entry
	// sr contains the static resolver
	sr *static.Resolver
	// platformID is the distribution platform id.
	platformID string
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
	// stateRoot is the .bldr state root dir.
	stateRoot string
	// vol is the volume used for state
	vol volume.Volume
	// peerID is the peerID to use for operations.
	peerID peer.ID
	// worldEngine is the world engine instance.
	worldEngine world.Engine
	// worldState is the world state instance.
	worldState world.WorldState
	// rels are the release funcs
	rels []func()
}

// BuildDistBus builds the storage and bus for the distribution entrypoint.
// Returns a set of functions to call to release the controllers.
func BuildDistBus(rctx context.Context, le *logrus.Entry, projectID, platformID, stateRoot string) (*DistBus, error) {
	le.Info("initializing application and storage...")
	ctx, ctxCancel := context.WithCancel(rctx)
	b, sr, err := NewCoreBus(ctx, le)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// add the configset controller
	configSetCtrl, _ := configset_controller.NewController(le, b)
	_, err = b.AddController(ctx, configSetCtrl, nil)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// build the plugin state paths on disk
	pluginHostObjectKey := "plugin-host"
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

	// build storage config
	storageMethods := default_storage.BuildStorage(b, stateRoot)
	if len(storageMethods) == 0 {
		ctxCancel()
		return nil, errors.New("no available storage methods")
	}

	// load storage
	storageMethod := storageMethods[0]
	storageMethod.AddFactories(b, sr)
	stConf := storageMethod.BuildVolumeConfig(projectID, &volume_controller.Config{
		VolumeIdAlias: []string{"dist"},
	})

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
	_, _, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, false, nil)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// start world
	engineID := "entrypoint"
	engineBucketID := engineID
	engineObjStoreID := engineBucketID

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

	distTransformConf := buildStorageTransformConf(projectID)
	transformConf, err := block_transform.NewConfig(distTransformConf)
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

	// register the world operation types for manifests
	lookupOpCtrl := world.NewLookupOpController("bldr-manifest-ops", engineID, bldr_manifest_world.LookupOp)
	relLookupCtrl, err := b.AddController(ctx, lookupOpCtrl, nil)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// ensure the manifest store exists in the world
	engTx, err := eng.NewTransaction(true)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	_, err = bldr_manifest_world.CreateManifestStore(ctx, engTx, pluginHostObjectKey)
	if err != nil {
		engTx.Discard()
		ctxCancel()
		return nil, err
	}

	if err := engTx.Commit(ctx); err != nil {
		ctxCancel()
		return nil, err
	}

	// build the plugin host controller
	pluginHostProcessConf := host_process.NewConfig(
		engineID,
		pluginHostObjectKey,
		vol.GetID(),
		vol.GetPeerID(),
		false, // run FetchManifest only if a manifest does not exist.
		pluginsStateRoot,
		pluginsDistRoot,
	)
	pluginHostCtrlObj, _, pluginHostRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(pluginHostProcessConf),
		ctxCancel,
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}
	pluginHostCtrl := pluginHostCtrlObj.(*plugin_host_controller.Controller)

	return &DistBus{
		ctx:                 ctx,
		b:                   b,
		le:                  le,
		sr:                  sr,
		platformID:          platformID,
		worldEngineID:       engineID,
		engineBucketID:      engineBucketID,
		engineObjectStoreID: engineObjStoreID,
		pluginHostObjectKey: pluginHostObjectKey,
		pluginHostCtrl:      pluginHostCtrl,
		st:                  storageMethod,
		stConf:              stConf,
		stateRoot:           stateRoot,
		vol:                 vol,
		peerID:              vol.GetPeerID(),
		worldEngine:         eng,
		worldState:          worldState,
		rels: []func(){
			pluginHostRef.Release,
			worldCtrlRef.Release,
			nodeCtrlRef.Release,
			relLookupCtrl,
			ctxCancel,
			diRef.Release,
			func() { volCtrl.Close() },
		},
	}, nil
}

// GetContext returns the context.
func (d *DistBus) GetContext() context.Context {
	return d.ctx
}

// GetBus returns the bus.
func (d *DistBus) GetBus() bus.Bus {
	return d.b
}

// GetLogger returns the root logger
func (d *DistBus) GetLogger() *logrus.Entry {
	return d.le
}

// GetStaticResolver returns the static controller resolver.
func (d *DistBus) GetStaticResolver() *static.Resolver {
	return d.sr
}

// GetDistPlatformID returns the distribution platform id.
func (d *DistBus) GetDistPlatformID() string {
	return d.platformID
}

// GetStateRoot returns the root of the state tree.
func (d *DistBus) GetStateRoot() string {
	return d.stateRoot
}

// GetStorage returns the storage.
func (d *DistBus) GetStorage() storage.Storage {
	return d.st
}

// GetStorageConf returns the storage config.
func (d *DistBus) GetStorageConf() config.Config {
	return d.stConf
}

// GetVolume returns the storage volume in use.
func (d *DistBus) GetVolume() volume.Volume {
	return d.vol
}

// GetWorldEngineID returns the world engine id.
func (d *DistBus) GetWorldEngineID() string {
	return d.worldEngineID
}

// GetWorldEngine returns the world engine instance.
func (d *DistBus) GetWorldEngine() world.Engine {
	return d.worldEngine
}

// GetWorldState returns the world state handle.
func (d *DistBus) GetWorldState() world.WorldState {
	return d.worldState
}

// GetPluginHostObjectKey returns the object key for the plugin host.
func (d *DistBus) GetPluginHostObjectKey() string {
	return d.pluginHostObjectKey
}

// Release releases the devtool bus.
func (d *DistBus) Release() {
	for _, rel := range d.rels {
		rel()
	}
}
