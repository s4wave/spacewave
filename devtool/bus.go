package devtool

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aperturerobotics/bifrost/peer"
	bldr "github.com/aperturerobotics/bldr"
	"github.com/aperturerobotics/bldr/core"
	core_devtool "github.com/aperturerobotics/bldr/core/devtool"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_project "github.com/aperturerobotics/bldr/project"
	bldr_project_controller "github.com/aperturerobotics/bldr/project/controller"
	bldr_project_watcher "github.com/aperturerobotics/bldr/project/watcher"
	default_storage "github.com/aperturerobotics/bldr/storage/default"
	storage_volume "github.com/aperturerobotics/bldr/storage/volume"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	"github.com/aperturerobotics/hydra/bucket"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/aperturerobotics/hydra/volume"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/aperturerobotics/util/exec"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.org/x/mod/modfile"
)

// devtoolTransformConf is the block transform conf to use.
var devtoolTransformConf = []config.Config{
	&transform_s2.Config{},
}

// distGoMod is the go mod path to use for the distribution bundle.
const distGoMod = "github.com/aperturerobotics/bldr-dist"

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
	// worldEngineID is the world engine id for the devtool world
	worldEngineID string
	// engineBucketID is the bucket used for world engine state storage
	engineBucketID string
	// engineObjectStoreID is the bucket used for root world engine state ref
	engineObjectStoreID string
	// pluginHostObjectKey is the object key used for the PluginHost
	pluginHostObjectKey string
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
	// rels are the release funcs
	rels []func()
}

// BuildDevtoolBus builds the storage and bus for the devtool.
// Returns a set of functions to call to release the controllers.
func BuildDevtoolBus(rctx context.Context, le *logrus.Entry, stateRoot string, watch bool) (*DevtoolBus, error) {
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
	)
	// engConf.Verbose = false
	engConf.Verbose = true
	engConf.DisableChangelog = true

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

	// distSrcDir is the path to the dist sources dir
	distSrcDir := filepath.Join(stateRoot, "src")

	return &DevtoolBus{
		ctx:                 ctx,
		b:                   b,
		le:                  le,
		sr:                  sr,
		watch:               watch,
		worldEngineID:       engineID,
		engineBucketID:      engineBucketID,
		engineObjectStoreID: engineObjStoreID,
		pluginHostObjectKey: pluginHostObjectKey,
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
		rels:                rels,
	}, nil
}

// SyncDistSources syncs the bldr sources and runs npm i and go mod vendor.
//
// bldrSum can be empty
// bldrSrcPath can be empty
func (d *DevtoolBus) SyncDistSources(bldrVersion, bldrSum, bldrSrcPath string) error {
	// mount the entrypoint web sources fsHandle
	ctx, le := d.ctx, d.le
	distSourcesHandle := bldr.BuildDistSourcesFSHandle(ctx, le)
	defer distSourcesHandle.Release()

	// sync the entrypoint sources to the path
	err := os.MkdirAll(d.distSrcRoot, 0o755)
	if err != nil {
		return err
	}
	err = unixfs_sync.Sync(
		ctx,
		d.distSrcRoot,
		distSourcesHandle,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		unixfs_sync.NewSkipPathPrefixes([]string{"vendor", "node_modules"}),
	)
	if err != nil {
		return err
	}

	runGoMod := func(cmd string) error {
		le.Infof("bldr sources: running go mod %s", cmd)
		goVendorCmd := exec.NewCmd("go", "mod", cmd)
		goVendorCmd.Dir = d.distSrcRoot
		goVendorCmd.Stderr = os.Stderr
		goVendorCmd.Stdout = os.Stderr
		goVendorCmd.Env = os.Environ()
		return goVendorCmd.Run()
	}

	// parse modfile
	bldrGoModPath := filepath.Join(d.distSrcRoot, "go.mod")
	bldrGoModData, err := os.ReadFile(bldrGoModPath)
	if err != nil {
		return err
	}
	bldrModFile, err := modfile.Parse(bldrGoModPath, bldrGoModData, nil)
	if err != nil {
		return err
	}
	bldrModPath := bldrModFile.Module.Mod.Path
	bldrModFile.Module.Mod.Path = distGoMod

	// change the mod to bldr-dist
	if err := bldrModFile.AddModuleStmt(distGoMod); err != nil {
		return err
	}

	if bldrSrcPath != "" {
		// apply the relative path
		if err := bldrModFile.AddReplace(bldrModPath, "", bldrSrcPath, ""); err != nil {
			return err
		}
	} else {
		// add a require for bldr if using bldrVersion
		if err := bldrModFile.AddRequire(bldrModPath, bldrVersion); err != nil {
			return err
		}
	}

	bldrModFile.Cleanup()
	updatedBldrGoMod, err := bldrModFile.Format()
	if err != nil {
		return err
	}
	if err := os.WriteFile(bldrGoModPath, updatedBldrGoMod, 0o644); err != nil {
		return err
	}
	if bldrSum != "" {
		// go.sum expects the module hash and the go.mod hash.
		// we expect the go.mod to match the bldrGoModData above.
		// calculate the go.mod checksum
		goModSum := sha256.Sum256(bldrGoModData)
		goModInner := fmt.Sprintf("%x  %s\n", goModSum, "go.mod")
		goModInnerSum := sha256.Sum256([]byte(goModInner))
		goModSumHash := "h1:" + base64.StdEncoding.EncodeToString(goModInnerSum[:])

		bldrGoSumPath := filepath.Join(d.distSrcRoot, "go.sum")
		goSumFile, err := os.OpenFile(bldrGoSumPath, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		_, err = goSumFile.WriteString(bldrModPath + " " + bldrVersion + " " + bldrSum + "\n")
		if err != nil {
			return err
		}
		_, err = goSumFile.WriteString(bldrModPath + " " + bldrVersion + "/go.mod " + goModSumHash + "\n")
		if err != nil {
			return err
		}
		if err = goSumFile.Close(); err != nil {
			return err
		}
	} else {
		if err := runGoMod("tidy"); err != nil {
			return err
		}
	}

	if err := runGoMod("vendor"); err != nil {
		return err
	}

	le.Info("done checking out bldr sources")

	return nil
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

// GetWorldEngine returns the world engine instance.
func (d *DevtoolBus) GetWorldEngine() world.Engine {
	return d.worldEngine
}

// GetWorldState returns the world state handle.
func (d *DevtoolBus) GetWorldState() world.WorldState {
	return d.worldState
}

// GetPluginHostObjectKey returns the object key for the plugin host.
func (d *DevtoolBus) GetPluginHostObjectKey() string {
	return d.pluginHostObjectKey
}

// StartProjectController reads the config file & starts the project controller.
// ConfigPath is the path to the project config.
// ConfigPath can be empty to start with an empty config.
// Returns the directive reference & controller.
func (d *DevtoolBus) StartProjectController(
	ctx context.Context,
	b bus.Bus,
	repoRoot,
	configPath string,
	startWithRemote string,
) (
	*bldr_project_watcher.Controller,
	directive.Reference,
	error,
) {
	absConfigPath := filepath.Join(repoRoot, configPath)
	projCtrlConf := bldr_project_controller.NewConfig(
		repoRoot,
		d.GetStateRoot(),
		&bldr_project.ProjectConfig{
			Remotes: map[string]*bldr_project.RemoteConfig{
				"devtool": {
					EngineId:       d.worldEngineID,
					PeerId:         d.peerID.String(),
					ObjectKey:      d.pluginHostObjectKey,
					LinkObjectKeys: []string{d.pluginHostObjectKey},
				},
			},
		},
		d.watch,
		startWithRemote != "",
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

	return ctrl.(*bldr_project_watcher.Controller), ctrlRef, nil
}

// Release releases the devtool bus.
func (d *DevtoolBus) Release() {
	for _, rel := range d.rels {
		rel()
	}
}
