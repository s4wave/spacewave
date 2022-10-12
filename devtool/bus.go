package devtool

import (
	"context"
	"os"
	"path"

	"github.com/aperturerobotics/bifrost/peer"
	bldr "github.com/aperturerobotics/bldr"
	"github.com/aperturerobotics/bldr/core"
	plugin_compiler "github.com/aperturerobotics/bldr/plugin/compiler"
	plugin_host "github.com/aperturerobotics/bldr/plugin/host"
	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	plugin_host_process "github.com/aperturerobotics/bldr/plugin/host/process"
	plugin_static "github.com/aperturerobotics/bldr/plugin/static"
	bldr_project "github.com/aperturerobotics/bldr/project"
	bldr_project_controller "github.com/aperturerobotics/bldr/project/controller"
	"github.com/aperturerobotics/bldr/storage"
	default_storage "github.com/aperturerobotics/bldr/storage/default"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/controller/resolver/static"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/controllerbus/util/exec"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	transform_s2 "github.com/aperturerobotics/hydra/block/transform/s2"
	"github.com/aperturerobotics/hydra/bucket"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/aperturerobotics/hydra/volume"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/pkg/errors"
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
	// stateRoot is the .bldr state root dir.
	stateRoot string
	// webSrcRoot is the path to the web entrypoint sources.
	webSrcRoot string
	// vol is the volume used for state
	vol volume.Volume
	// peer is the peer to use for operations.
	peer peer.Peer
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

	// add controller factories
	sr.AddFactory(world_block_engine.NewFactory(b))
	sr.AddFactory(plugin_host_process.NewFactory(b))
	sr.AddFactory(bldr_project_controller.NewFactory(b))
	sr.AddFactory(plugin_compiler.NewFactory(b))

	// add the configset controller
	configSetCtrl, _ := configset_controller.NewController(le, b)
	_, err = b.AddController(ctx, configSetCtrl, nil)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	// build the plugin state paths on disk
	pluginHostObjectKey := "devtool/plugin-host"
	pluginsRoot := stateRoot
	pluginsDistRoot := path.Join(pluginsRoot, "dist")
	if err := os.MkdirAll(pluginsDistRoot, 0755); err != nil {
		ctxCancel()
		return nil, err
	}
	pluginsStateRoot := path.Join(pluginsRoot, "state")
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

	// ensure the plugin host exists in the world
	engTx, err := eng.NewTransaction(true)
	if err != nil {
		ctxCancel()
		return nil, err
	}

	_, err = plugin_host.CreatePluginHost(ctx, engTx, pluginHostObjectKey)
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
	var vpeer peer.Peer = vol
	pluginHostProcessConf := host_process.NewConfig(
		engineID,
		pluginHostObjectKey,
		vpeer.GetPeerID(),
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

	// webSrcDir is the path to the web sources dir
	webSrcDir := path.Join(stateRoot, "bldr")
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
		stateRoot:           stateRoot,
		webSrcRoot:          webSrcDir,
		vol:                 vol,
		peer:                vpeer,
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

// SyncWebSources syncs the web/ sources and runs npm i and go mod vendor.
func (d *DevtoolBus) SyncWebSources() error {
	// mount the entrypoint web sources fsHandle
	ctx, le := d.ctx, d.le
	webSourcesHandle := bldr.BuildWebSourcesFSHandle(ctx, le)
	defer webSourcesHandle.Release()

	// sync the entrypoint sources to the path
	err := os.MkdirAll(d.webSrcRoot, 0755)
	if err != nil {
		return err
	}
	err = unixfs_sync.Sync(
		ctx,
		d.webSrcRoot,
		webSourcesHandle,
		unixfs_sync.DeleteMode_DeleteMode_DURING,
		[]string{"vendor", "node_modules"},
	)
	if err != nil {
		return err
	}

	// run go mod vendor
	le.Info("running go mod vendor for bldr sources")
	goVendorCmd := exec.NewCmd("go", "mod", "vendor")
	goVendorCmd.Dir = d.webSrcRoot
	goVendorCmd.Stderr = os.Stderr
	goVendorCmd.Stdout = os.Stderr
	goVendorCmd.Env = os.Environ()
	if err := goVendorCmd.Run(); err != nil {
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

// GetWebSrcDir returns the path to the web sources checked out under StateRoot.
func (d *DevtoolBus) GetWebSrcDir() string {
	return d.webSrcRoot
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

// ExecStaticPlugin executes the plugin static loader.
// Returns an error if the controller exited unsucessfully.
// If rplugin is nil, returns nil, nil
func (d *DevtoolBus) ExecStaticPlugin(
	ctx context.Context,
	le *logrus.Entry,
	info *controller.Info,
	rplugin *plugin_static.StaticPlugin,
) error {
	if rplugin == nil {
		return nil
	}

	conf := &plugin_static.Config{
		EngineId:      d.worldEngineID,
		PluginHostKey: d.pluginHostObjectKey,
		PeerId:        d.peer.GetPeerID().Pretty(),
	}
	ctrl := plugin_static.NewController(
		le,
		d.b,
		conf,
		info,
		rplugin,
	)
	return d.b.ExecuteController(ctx, ctrl)
}

// StartProjectController reads the config file & starts the project controller.
// ConfigPath is the path to the project config.
// ConfigPath can be empty to start with an empty config.
// Returns the directive reference & controller.
func (d *DevtoolBus) StartProjectController(
	ctx context.Context,
	b bus.Bus,
	startProject bool,
	repoRoot,
	configPath,
	platformID string,
) (
	controller.Controller,
	directive.Reference,
	error,
) {
	projConfig := &bldr_project.ProjectConfig{}
	if configPath != "" {
		projConfYaml, err := os.ReadFile(configPath)
		if err != nil {
			return nil, nil, err
		}
		if err := bldr_project.UnmarshalProjectConfig(projConfYaml, projConfig); err != nil {
			return nil, nil, errors.Wrap(err, "unmarshal project config")
		}
		if err := projConfig.Validate(); err != nil {
			return nil, nil, err
		}
	}

	ctrl, _, ctrlRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(bldr_project_controller.NewConfig(
			repoRoot,
			d.GetStateRoot(),
			projConfig,
			startProject,
			d.worldEngineID,
			d.peer.GetPeerID().Pretty(),
			d.GetPluginHostObjectKey(),
			platformID,
		)),
		nil,
	)
	if err != nil {
		return nil, nil, err
	}

	return ctrl, ctrlRef, nil
}

// Release releases the devtool bus.
func (d *DevtoolBus) Release() {
	for _, rel := range d.rels {
		rel()
	}
}
