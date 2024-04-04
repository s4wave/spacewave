package dist_entrypoint

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	plugin_host_default "github.com/aperturerobotics/bldr/plugin/host/default"
	default_storage "github.com/aperturerobotics/bldr/storage/default"
	storage_volume "github.com/aperturerobotics/bldr/storage/volume"
	"github.com/aperturerobotics/controllerbus/bus"
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
	// storageID is the id of the storage attached to the bus
	storageID string
	// worldEngineID is the world engine id for state
	worldEngineID string
	// engineBucketID is the bucket used for world engine state storage
	engineBucketID string
	// engineObjectStoreID is the bucket used for root world engine state ref
	engineObjectStoreID string
	// pluginHostObjectKey is the object key used for the PluginHost
	pluginHostObjectKey string
	// pluginHostCtrl is the plugin host controller
	pluginHostCtrl *plugin_host_default.PluginHostController
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
func BuildDistBus(
	rctx context.Context,
	le *logrus.Entry,
	projectID,
	platformID,
	stateRoot,
	webRuntimeID string,
) (*DistBus, error) {
	le.
		WithFields(logrus.Fields{"project-id": projectID, "platform-id": platformID}).
		Info("initializing application and storage...")
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
	pluginsRoot := filepath.Join(stateRoot, "p")
	pluginsDistRoot := filepath.Join(pluginsRoot, "d")
	pluginsStateRoot := filepath.Join(pluginsRoot, "s")

	// HACK: we cannot create paths on the web platform
	isWebPlatform := platformID == "web" || strings.HasPrefix(platformID, "web/")
	if !isWebPlatform {
		if err := os.MkdirAll(pluginsDistRoot, 0o755); err != nil {
			ctxCancel()
			return nil, err
		}
		if err := os.MkdirAll(pluginsStateRoot, 0o755); err != nil {
			ctxCancel()
			return nil, err
		}
	}

	// attach the default storage controller
	// this provides separate named volumes with the storage volume controller.
	storageID := default_storage.StorageID
	storageCtrl := default_storage.NewController(storageID, b, stateRoot)
	relStorageCtrl, err := b.AddController(ctx, storageCtrl, nil)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// ensure there is at least one storage method
	storageMethods := storageCtrl.GetStorage()
	if len(storageMethods) == 0 {
		ctxCancel()
		return nil, errors.New("no available storage methods")
	}

	// run the distribution storage volume (used for storing dist manifests)
	volCtrli, _, diRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&storage_volume.Config{
			StorageId:       storageID,
			StorageVolumeId: "dist/" + projectID,
			VolumeConfig: &volume_controller.Config{
				VolumeIdAlias: []string{"dist"},
			},
		}),
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
	_, _, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
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
	worldState := world.NewEngineWorldState(eng, true)

	// register the world operation types for manifests
	lookupOpCtrl := world.NewLookupOpController("bldr-manifest-ops", engineID, bldr_manifest_world.LookupOp)
	relLookupCtrl, err := b.AddController(ctx, lookupOpCtrl, nil)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// ensure the manifest store exists in the world
	if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, eng, pluginHostObjectKey); err != nil {
		ctxCancel()
		return nil, err
	}

	// build the plugin host controller
	pluginHostCtrl, pluginHostRel, err := plugin_host_default.StartBusPluginHost(
		ctx,
		b,
		engineID,
		pluginHostObjectKey,
		vol.GetID(),
		vol.GetPeerID().String(),
		pluginsStateRoot,
		pluginsDistRoot,
		true,
		false,
		webRuntimeID,
	)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	return &DistBus{
		ctx:                 ctx,
		b:                   b,
		le:                  le,
		sr:                  sr,
		storageID:           storageID,
		platformID:          platformID,
		worldEngineID:       engineID,
		engineBucketID:      engineBucketID,
		engineObjectStoreID: engineObjStoreID,
		pluginHostObjectKey: pluginHostObjectKey,
		pluginHostCtrl:      pluginHostCtrl,
		stateRoot:           stateRoot,
		vol:                 vol,
		peerID:              vol.GetPeerID(),
		worldEngine:         eng,
		worldState:          worldState,
		rels: []func(){
			relStorageCtrl,
			pluginHostRel,
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

// GetStorageID returns the storag eid.
func (d *DistBus) GetStorageID() string {
	return d.storageID
}

// GetDistPlatformID returns the distribution platform id.
func (d *DistBus) GetDistPlatformID() string {
	return d.platformID
}

// GetStateRoot returns the root of the state tree.
func (d *DistBus) GetStateRoot() string {
	return d.stateRoot
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
