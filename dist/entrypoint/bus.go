package dist_entrypoint

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/bifrost/peer"
	bldr_dist "github.com/aperturerobotics/bldr/dist"
	manifest_fetch_world "github.com/aperturerobotics/bldr/manifest/fetch/world"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host_default "github.com/aperturerobotics/bldr/plugin/host/default"
	plugin_host_scheduler "github.com/aperturerobotics/bldr/plugin/host/scheduler"
	default_storage "github.com/aperturerobotics/bldr/storage/default"
	storage_volume "github.com/aperturerobotics/bldr/storage/volume"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller/configset"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/go-kvfile"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/bucket"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	store_kvkey "github.com/aperturerobotics/hydra/store/kvkey"
	"github.com/aperturerobotics/hydra/volume"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/aperturerobotics/util/refcount"
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
	// pluginSchedCtrl is the plugin scheduler
	pluginSchedCtrl *plugin_host_scheduler.Controller
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
	// rel is the release func
	rel func()
}

// BuildDistBus builds the storage and bus for the distribution entrypoint.
// Returns a set of functions to call to release the controllers.
func BuildDistBus(
	rctx context.Context,
	le *logrus.Entry,
	distMeta *bldr_dist.DistMeta,
	stateRoot,
	webRuntimeID string,
	configSetProto *configset_proto.ConfigSet,
	staticBlockStoreReaderBuilder refcount.RefCountResolver[*kvfile.Reader],
) (*DistBus, error) {
	projectID := distMeta.GetProjectId()
	platformID := distMeta.GetPlatformId()
	le.
		WithFields(logrus.Fields{"project-id": projectID, "platform-id": platformID}).
		Info("initializing application and storage...")
	ctx, ctxCancel := context.WithCancel(rctx)

	rels := []func(){ctxCancel}
	rel := func() {
		for _, rel := range rels {
			if rel != nil {
				rel()
			}
		}
	}

	b, sr, err := NewCoreBus(ctx, le)
	if err != nil {
		rel()
		return nil, err
	}

	// add the configset controller
	configSetCtrl, _ := configset_controller.NewController(le, b)
	_, err = b.AddController(ctx, configSetCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}

	// build the plugin state paths on disk
	pluginsRoot := filepath.Join(stateRoot, "p")
	pluginsDistRoot := filepath.Join(pluginsRoot, "d")
	pluginsStateRoot := filepath.Join(pluginsRoot, "s")

	// HACK: we cannot create paths on the web platform
	isWebPlatform := platformID == "js" || strings.HasPrefix(platformID, "native/js/")
	if !isWebPlatform {
		if err := os.MkdirAll(pluginsDistRoot, 0o755); err != nil {
			rel()
			return nil, err
		}
		if err := os.MkdirAll(pluginsStateRoot, 0o755); err != nil {
			rel()
			return nil, err
		}
	}

	// run the config set
	if len(configSetProto.GetConfigs()) != 0 {
		configSet, err := configSetProto.Resolve(ctx, b)
		if err != nil {
			rel()
			return nil, err
		}

		if len(configSet) != 0 {
			_, applyCsetRef, err := b.AddDirective(configset.NewApplyConfigSet(configSet), nil)
			if err != nil {
				rel()
				return nil, err
			}
			rels = append(rels, applyCsetRef.Release)
		}
	}

	// attach the default storage controller
	// this provides separate named volumes with the storage volume controller.
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

	// add storage factories
	for _, st := range storageMethods {
		st.AddFactories(b, sr)
	}

	distBundleWorldRootRef := distMeta.GetDistWorldRef()
	distBundleObjKey := distMeta.GetDistObjectKey()

	// mount the embedded read-only block storage
	embedBlockStoreID := bldr_dist.StaticBlockStoreID
	staticBlockStoreCtrl := NewStaticBlockStore(
		le,
		b,
		embedBlockStoreID,
		staticBlockStoreReaderBuilder,
		store_kvkey.NewDefaultKVKey(),
		nil, // []string{distBundleBucketConf.GetId()},
		nil,
	)
	relStaticVolCtrl, err := b.AddController(ctx, staticBlockStoreCtrl, nil)
	if err != nil {
		rel()
		return nil, errors.Wrap(err, "add static block store controller")
	}
	rels = append(rels, relStaticVolCtrl)

	// run the distribution storage volume
	// used for the plugin host world
	volCtrli, _, diRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&storage_volume.Config{
			StorageId:       storageID,
			StorageVolumeId: "dist/" + projectID,
			VolumeConfig: &volume_controller.Config{
				VolumeIdAlias: []string{"dist"},

				DisableEventBlockRm:     true,
				DisableReconcilerQueues: true,
				DisablePeer:             true,
			},
		}),
		ctxCancel,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, diRef.Release)

	volCtrl, ok := volCtrli.(volume.Controller)
	if !ok {
		rel()
		return nil, errors.New("volume controller returned invalid value")
	}

	vol, err := volCtrl.GetVolume(ctx)
	if err != nil {
		rel()
		return nil, err
	}

	// apply the dist bucket config to the node storage
	// note: make sure this matches dist compiler at create the embedded manifests world part
	distBundleBucketConf, err := bldr_dist.NewDistBucketConfig(projectID)
	if err != nil {
		rel()
		return nil, err
	}
	_, _, _, err = vol.ApplyBucketConfig(ctx, distBundleBucketConf)
	if err != nil {
		rel()
		return nil, err
	}

	// mount the manifest kvtx block world backed by read-only storage
	distWorldEngineID := bldr_dist.DistWorldEngineID
	embedEngineConf := world_block_engine.NewConfig(
		distWorldEngineID,
		vol.GetID(),
		distBundleBucketConf.GetId(),
		"",
		distBundleWorldRootRef,
		nil,
		false,
	)
	embedEngineConf.DisableLookup = true

	_, _, embedEngineCtrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(embedEngineConf),
		nil,
	)
	if err != nil {
		rel()
		return nil, errors.Wrap(err, "start static embedded engine controller")
	}
	rels = append(rels, embedEngineCtrlRef.Release)

	// mount the manifest fetcher from the static world
	staticManifestFetcher := manifest_fetch_world.NewController(le, b, &manifest_fetch_world.Config{
		EngineId:   distWorldEngineID,
		ObjectKeys: []string{distBundleObjKey},
	})
	relStaticManifestFetcher, err := b.AddController(ctx, staticManifestFetcher, nil)
	if err != nil {
		rel()
		return nil, errors.Wrap(err, "start static manifest fetcher")
	}
	rels = append(rels, relStaticManifestFetcher)

	// start the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, nodeCtrlRef.Release)

	// start world
	engineID := "entrypoint/" + projectID
	engineBucketID := engineID
	engineObjStoreID := engineBucketID

	// create bucket if it doesn't exist
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

	distTransformConf := buildStorageTransformConf(projectID)
	transformConf, err := block_transform.NewConfig(distTransformConf)
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
		vol.GetID(),
		engineBucketID,
		engineObjStoreID,
		initRef,
		nil,
		false,
	)
	engConf.DisableLookup = true

	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		b,
		engConf,
	)
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

	// register the world operation types for manifests
	lookupOpCtrl := world.NewLookupOpController("bldr-manifest-ops", engineID, bldr_manifest_world.LookupOp)
	relLookupCtrl, err := b.AddController(ctx, lookupOpCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relLookupCtrl)

	// ensure the manifest store exists in the world
	pluginHostObjectKey := "plugin-host"
	if _, err := bldr_manifest_world.CreateManifestStoreInEngine(ctx, eng, pluginHostObjectKey); err != nil {
		rel()
		return nil, err
	}

	// build the plugin scheduler
	pluginSchedCtrl, pluginSchedCtrlRel, err := plugin_host_default.StartPluginScheduler(
		ctx,
		b,
		engineID,
		pluginHostObjectKey,
		vol.GetID(),
		vol.GetPeerID().String(),
		true,  // Watch FetchManifest on the bus so we can do auto-update via plugins.
		false, // Enable copying the manifest root to the plugin host storage.

		// Enable copying the manifest contents to the plugin host storage.
		//
		// This is particularly necessary since the plugin that provided the
		// manifest might exit before being restarted, thereby creating a
		// situation where we depend on that plugin for the data to start it,
		// but that plugin is not running, so nothing happens (stuck).
		false,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, pluginSchedCtrlRel)

	// build the plugin host controller
	pluginHostCtrl, pluginHostRel, err := plugin_host_default.StartPluginHost(
		ctx,
		b,
		pluginsStateRoot,
		pluginsDistRoot,
		webRuntimeID,
	)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, pluginHostRel)

	// Create LoadPlugin directives for the startup plugins.
	for _, pluginID := range distMeta.GetStartupPlugins() {
		_, pluginRef, err := b.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
		if err != nil {
			le.WithError(err).WithField("plugin-id", pluginID).Warn("failed to load startup plugin")
			continue
		}
		rels = append(rels, pluginRef.Release)
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
		pluginSchedCtrl:     pluginSchedCtrl,
		pluginHostCtrl:      pluginHostCtrl,
		stateRoot:           stateRoot,
		vol:                 vol,
		peerID:              vol.GetPeerID(),
		worldEngine:         eng,
		worldState:          worldState,
		rel:                 rel,
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
	d.rel()
}
