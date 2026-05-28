//go:build !js

package devtool

import (
	"context"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	bldr "github.com/s4wave/spacewave/bldr"
	"github.com/s4wave/spacewave/bldr/core"
	core_devtool "github.com/s4wave/spacewave/bldr/core/devtool"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	bldr_project_controller "github.com/s4wave/spacewave/bldr/project/controller"
	bldr_project_watcher "github.com/s4wave/spacewave/bldr/project/watcher"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_s2 "github.com/s4wave/spacewave/db/block/transform/s2"
	"github.com/s4wave/spacewave/db/bucket"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/peer"
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
	// watch enables watching for changes
	watch bool
	// storageID is the storage engine id
	storageID string
	// worldEngineID is the world engine id for the devtool world
	worldEngineID string
	// engineBucketID is the bucket used for world engine state storage
	engineBucketID string
	// engineObjectStoreID is the bucket used for root world engine state ref
	engineObjectStoreID string
	// pluginHostObjectKey is the object key used for the PluginHost
	pluginHostObjectKey string
	// repoRoot is the project root containing the live module files.
	repoRoot string
	// stateRoot is the .bldr state root dir.
	stateRoot string
	// distSrcRoot is the path to the web entrypoint sources.
	distSrcRoot string
	// pluginsDistRoot is the path to the plugins dist dir.
	pluginsDistRoot string
	// pluginsStateRoot is the path to the plugins state dir.
	pluginsStateRoot string
	// vol is the volume used for state
	vol volume.Volume
	// volInfo is the volume info for the vol used for state
	volInfo *volume.VolumeInfo
	// volCtrl is the volume controller used for state
	volCtrl volume.Controller
	// peerID is the peerID to use for operations.
	peerID peer.ID
	// worldEngine is the world engine instance.
	worldEngine world.Engine
	// worldState is the world state instance.
	worldState world.WorldState
	// statusProducer publishes devtool status snapshots.
	statusProducer *devtool_status.BldrDevtoolStatusProducer
	// rels are the release funcs
	rels []func()
}

// BuildDevtoolBus builds the storage and bus for the devtool.
// Returns a set of functions to call to release the controllers.
func BuildDevtoolBus(
	rctx context.Context,
	le *logrus.Entry,
	repoRoot, stateRoot string,
	watch bool,
) (*DevtoolBus, error) {
	ctx, ctxCancel := context.WithCancel(rctx)
	var rels []func()
	rel := func() {
		for _, fn := range rels {
			fn()
		}
		ctxCancel()
	}

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		rel()
		return nil, err
	}

	statusProducer := devtool_status.NewBldrDevtoolStatusProducer(nil)
	statusObserver := devtool_status.NewBldrDevtoolStatusObserver(b, statusProducer)
	relStatusObserver, err := b.AddController(ctx, statusObserver, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relStatusObserver)

	// add controller factories
	core_devtool.AddFactories(b, sr)

	// add the configset controller
	configSetCtrl, _ := configset_controller.NewController(le, b)
	relConfigSetCtrl, err := b.AddController(ctx, configSetCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relConfigSetCtrl)

	// build the plugin state paths on disk
	pluginHostObjectKey := "devtool"
	pluginsRoot := filepath.Join(stateRoot, "plugin")
	pluginsDistRoot := filepath.Join(pluginsRoot, "dist")
	if err := os.MkdirAll(pluginsDistRoot, 0o755); err != nil {
		rel()
		return nil, err
	}
	pluginsStateRoot := filepath.Join(pluginsRoot, "state")
	if err := os.MkdirAll(pluginsStateRoot, 0o755); err != nil {
		rel()
		return nil, err
	}

	// add storage factories

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
		ctxCancel()
		return nil, errors.New("no available storage methods")
	}

	// add the storage method factories
	for _, storageMethod := range storageMethods {
		storageMethod.AddFactories(b, sr)
	}

	volCtrl, volCtrlRef, err := storage_volume.ExecVolumeController(ctx, b, &storage_volume.Config{
		StorageId:       storageID,
		StorageVolumeId: "devtool",
		VolumeConfig: &volume_controller.Config{
			VolumeIdAlias: []string{"dist"},
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

	volInfo, err := volume.NewVolumeInfo(ctx, volCtrl.GetControllerInfo(), vol)
	if err != nil {
		rel()
		return nil, err
	}

	// start the node controller.
	dir := resolver.NewLoadControllerWithConfig(&node_controller.Config{})
	_, _, nodeCtrlRef, err := bus.ExecOneOff(ctx, b, dir, nil, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, nodeCtrlRef.Release)

	// start devtool world
	engineBucketID := "bldr/devtool"
	engineObjStoreID := engineBucketID
	engineID := "bldr"

	// create bucket if it doesn't exist
	bucketConf, err := bucket.NewConfig(engineBucketID, 1, nil)
	if err != nil {
		rel()
		return nil, err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(bucketConf, vol.GetID()))
	if err != nil {
		rel()
		return nil, err
	}

	// Register GC hierarchy: gcroot -> bucket
	if rg := vol.GetRefGraph(); rg != nil {
		if err := block_gc.RegisterEntityChain(ctx, rg,
			block_gc.NodeGCRoot,
			block_gc.BucketIRI(engineBucketID),
		); err != nil {
			rel()
			return nil, err
		}
	}

	transformConf, err := block_transform.NewConfig(devtoolTransformConf)
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
	// engConf.Verbose = true

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

	// register the world operation types for plugin host
	lookupOpCtrl := world.NewLookupOpController("bldr-plugin-host-ops", engineID, bldr_manifest_world.LookupOp)
	relLookupCtrl, err := b.AddController(ctx, lookupOpCtrl, nil)
	if err != nil {
		rel()
		return nil, err
	}
	rels = append(rels, relLookupCtrl)

	// ensure the plugin host exists in the world
	engTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		rel()
		return nil, err
	}

	_, err = bldr_manifest_world.CreateManifestStore(ctx, engTx, pluginHostObjectKey)
	if err != nil {
		engTx.Discard()
		rel()
		return nil, err
	}

	if err := engTx.Commit(ctx); err != nil {
		rel()
		return nil, err
	}

	// Inspect existing devtool manifests. Native startup now preflights cached
	// manifests before the scheduler starts, so startup keeps validated cached
	// manifests instead of deleting everything up front.
	inspectTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		rel()
		return nil, err
	}
	manifestObjKeys, err := bldr_manifest_world.ListManifests(ctx, inspectTx, pluginHostObjectKey)
	if err != nil {
		inspectTx.Discard()
		rel()
		return nil, err
	}
	if len(manifestObjKeys) > 0 {
		le.WithField("manifest-count", len(manifestObjKeys)).
			Info("keeping cached devtool manifests for startup validation")
	}
	inspectTx.Discard()

	// distSrcDir is the path to the dist sources dir
	distSrcDir := filepath.Join(stateRoot, "src")

	return &DevtoolBus{
		ctx:                 ctx,
		b:                   b,
		le:                  le,
		sr:                  sr,
		watch:               watch,
		storageID:           storageID,
		worldEngineID:       engineID,
		engineBucketID:      engineBucketID,
		engineObjectStoreID: engineObjStoreID,
		pluginHostObjectKey: pluginHostObjectKey,
		repoRoot:            repoRoot,
		stateRoot:           stateRoot,
		distSrcRoot:         distSrcDir,
		pluginsDistRoot:     pluginsDistRoot,
		pluginsStateRoot:    pluginsStateRoot,
		vol:                 vol,
		volInfo:             volInfo,
		volCtrl:             volCtrl,
		peerID:              vol.GetPeerID(),
		worldEngine:         eng,
		worldState:          worldState,
		statusProducer:      statusProducer,
		rels:                rels,
	}, nil
}

// SyncDistSources syncs the bldr sources and runs npm i and go mod vendor.
//
// bldrSum can be empty
// bldrSrcPath can be empty
func (d *DevtoolBus) SyncDistSources(bldrVersion, bldrSum, bldrSrcPath string) error {
	return bldr.SyncDistSources(d.ctx, d.le, bldr.DistSourceSyncConfig{
		RepoRoot:    d.repoRoot,
		DistRoot:    d.distSrcRoot,
		BldrVersion: bldrVersion,
		BldrSum:     bldrSum,
		BldrSrcPath: bldrSrcPath,
	})
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

// GetStateRoot returns the root of the state tree.
func (d *DevtoolBus) GetStateRoot() string {
	return d.stateRoot
}

// GetDistSrcDir returns the path to the redistribute sources checked out under StateRoot.
func (d *DevtoolBus) GetDistSrcDir() string {
	return d.distSrcRoot
}

// GetPluginsDistRoot returns the path to the plugins dist files dir.
func (d *DevtoolBus) GetPluginsDistRoot() string {
	return d.pluginsDistRoot
}

// GetPluginsStateRoot returns the path to the plugins state files dir.
func (d *DevtoolBus) GetPluginsStateRoot() string {
	return d.pluginsStateRoot
}

// GetVolume returns the storage volume in use.
func (d *DevtoolBus) GetVolume() volume.Volume {
	return d.vol
}

// GetVolumeInfo returns the storage volume info.
func (d *DevtoolBus) GetVolumeInfo() *volume.VolumeInfo {
	return d.volInfo
}

// GetVolumeController returns the storage volume controller in use.
func (d *DevtoolBus) GetVolumeController() volume.Controller {
	return d.volCtrl
}

// GetWorldEngineID returns the world engine id.
func (d *DevtoolBus) GetWorldEngineID() string {
	return d.worldEngineID
}

// GetStorageID returns the storage controller id.
func (d *DevtoolBus) GetStorageID() string {
	return d.storageID
}

// GetWorldEngine returns the world engine instance.
func (d *DevtoolBus) GetWorldEngine() world.Engine {
	return d.worldEngine
}

// GetWorldState returns the world state handle.
func (d *DevtoolBus) GetWorldState() world.WorldState {
	return d.worldState
}

// GetStatusProducer returns the devtool status producer.
func (d *DevtoolBus) GetStatusProducer() *devtool_status.BldrDevtoolStatusProducer {
	return d.statusProducer
}

// SetCommandStatus publishes the current devtool command lifecycle status.
func (d *DevtoolBus) SetCommandStatus(command devtool_status.BldrDevtoolCommandStatus) {
	d.statusProducer.UpdateStatus(func(current *devtool_status.BldrDevtoolStatus) *devtool_status.BldrDevtoolStatus {
		return current.WithCommand(command)
	})
}

// GetPluginHostObjectKey returns the object key for the plugin host.
func (d *DevtoolBus) GetPluginHostObjectKey() string {
	return d.pluginHostObjectKey
}

// StartStorageVolume starts a storage volume.
// The ID should be unique.
func (d *DevtoolBus) StartStorageVolume(
	ctx context.Context,
	storageVolumeID string,
	ctrlConf *volume_controller.Config,
) (volume.Controller, directive.Reference, error) {
	return storage_volume.ExecVolumeController(ctx, d.GetBus(), &storage_volume.Config{
		StorageId:       d.storageID,
		StorageVolumeId: storageVolumeID,
		VolumeConfig:    ctrlConf,
	})
}

// StartProjectController reads the config file & starts the project controller.
// ConfigPath is the path to the project config.
// ConfigPath can be empty to start with an empty config.
// extraPlugins are additional plugin IDs appended to the start config.
// Returns the directive reference & controller.
func (d *DevtoolBus) StartProjectController(
	ctx context.Context,
	b bus.Bus,
	repoRoot,
	configPath string,
	startWithRemote string,
	extraPlugins []string,
) (
	*bldr_project_watcher.Controller,
	directive.Reference,
	error,
) {
	return d.StartProjectControllerWithStartup(
		ctx,
		b,
		repoRoot,
		configPath,
		startWithRemote,
		extraPlugins,
		startWithRemote != "",
	)
}

// StartProjectControllerWithStartup reads the config file and starts the project controller.
func (d *DevtoolBus) StartProjectControllerWithStartup(
	ctx context.Context,
	b bus.Bus,
	repoRoot,
	configPath string,
	startWithRemote string,
	extraPlugins []string,
	start bool,
) (
	*bldr_project_watcher.Controller,
	directive.Reference,
	error,
) {
	absConfigPath := filepath.Join(repoRoot, configPath)

	// Validate the config file upfront so parse errors surface immediately
	// instead of causing the controller to retry indefinitely.
	// bldr.star is also accepted as a standalone config source.
	if absConfigPath != "" {
		confData, err := os.ReadFile(absConfigPath)
		if err != nil {
			// If bldr.yaml is missing, check for bldr.star.
			starPath := bldr_project_watcher.ResolveStarlarkPath(absConfigPath)
			if _, serr := os.Stat(starPath); serr != nil {
				return nil, nil, errors.Wrap(err, "read project config")
			}
		} else {
			testConf := &bldr_project.ProjectConfig{}
			if err := bldr_project.UnmarshalProjectConfig(confData, testConf); err != nil {
				return nil, nil, errors.Wrap(err, "parse project config")
			}
		}
	}

	baseProjectConfig := &bldr_project.ProjectConfig{
		Remotes: map[string]*bldr_project.RemoteConfig{
			"devtool": {
				EngineId:       d.worldEngineID,
				PeerId:         d.peerID.String(),
				ObjectKey:      d.pluginHostObjectKey,
				LinkObjectKeys: []string{d.pluginHostObjectKey},
			},
		},
	}
	if len(extraPlugins) != 0 {
		baseProjectConfig.Start = &bldr_project.StartConfig{
			Plugins: extraPlugins,
		}
	}
	projCtrlConf := bldr_project_controller.NewConfig(
		repoRoot,
		d.GetStateRoot(),
		baseProjectConfig,
		d.watch,
		start,
	)
	projCtrlConf.FetchManifestRemote = startWithRemote
	projWatcherConfig := &bldr_project_watcher.Config{
		ConfigPath:              absConfigPath, //   configPath,
		DisableWatch:            !d.watch,
		ProjectControllerConfig: projCtrlConf,
	}

	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(projWatcherConfig),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	projWatcher := ctrl.(*bldr_project_watcher.Controller)
	projCtrl, err := projWatcher.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		ctrlRef.Release()
		return nil, nil, err
	}
	devtool_status.AttachManifestBuildStatus(d.statusProducer, projCtrl)
	return projWatcher, ctrlRef, nil
}

// Release releases the devtool bus.
func (d *DevtoolBus) Release() {
	for _, rel := range d.rels {
		rel()
	}
}
