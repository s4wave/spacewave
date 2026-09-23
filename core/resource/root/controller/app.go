package resource_root_controller

import (
	"context"
	"net/http"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	bus_bridge "github.com/aperturerobotics/controllerbus/bus/bridge"
	"github.com/aperturerobotics/controllerbus/config"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/ulid"
	bldr_core "github.com/s4wave/spacewave/bldr/core"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host "github.com/s4wave/spacewave/bldr/plugin/host"
	plugin_host_configset "github.com/s4wave/spacewave/bldr/plugin/host/configset"
	plugin_host_root "github.com/s4wave/spacewave/bldr/plugin/host/root"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	plugin_host_storage_volume "github.com/s4wave/spacewave/bldr/plugin/host/storage/volume"
	"github.com/s4wave/spacewave/bldr/storage"
	storage_controller "github.com/s4wave/spacewave/bldr/storage/controller"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	storage_world "github.com/s4wave/spacewave/bldr/storage/world"
	plugin_space "github.com/s4wave/spacewave/core/plugin/space"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	resource_root "github.com/s4wave/spacewave/core/resource/root"
	session_controller "github.com/s4wave/spacewave/core/session/controller"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	space_http_download "github.com/s4wave/spacewave/core/space/http/download"
	space_http_export "github.com/s4wave/spacewave/core/space/http/export"
	space_sobject "github.com/s4wave/spacewave/core/space/sobject"
	space_world_blocktype "github.com/s4wave/spacewave/core/space/world/blocktype"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/bucket"
	db_core "github.com/s4wave/spacewave/db/core"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	object_peer "github.com/s4wave/spacewave/db/object/peer"
	"github.com/s4wave/spacewave/db/volume"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_kvtxinmem "github.com/s4wave/spacewave/db/volume/kvtxinmem"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	bifrost_http "github.com/s4wave/spacewave/net/http"
)

const appBackingEngineID = "app-backing-world"

// newAppRegistry shares one runtime among live attachments to the same binding.
// The enclosing controller's context owns all runtime construction and shutdown.
func (c *Controller) newAppRegistry() *keyed.KeyedRefCount[resource_root.AppStorage, *promise.Promise[*appRuntime]] {
	return keyed.NewKeyedRefCount(func(binding resource_root.AppStorage) (keyed.Routine, *promise.Promise[*appRuntime]) {
		ready := promise.NewPromise[*appRuntime]()
		return func(ctx context.Context) error {
			runtime, err := c.buildApp(ctx, binding)
			ready.SetResult(runtime, err)
			if err != nil {
				return err
			}
			defer runtime.close()
			<-ctx.Done()
			return nil
		}, ready
	})
}

// mountApp retains the shared runtime until the caller releases its Resource.
func (c *Controller) mountApp(ctx context.Context, binding resource_root.AppStorage) (resource_root.AppRuntime, func(), error) {
	ref, ready, _ := c.apps.AddKeyRef(binding)
	runtime, err := ready.Await(ctx)
	if err != nil {
		ref.Release()
		return nil, nil, err
	}
	return runtime, func() {
		ref.Release()
		if current, exists := c.apps.GetKey(binding); !exists || current != ready {
			<-runtime.done
		}
	}, nil
}

// appRuntime owns the child bus and ordinary root controller.
type appRuntime struct {
	ctx      context.Context
	cancel   context.CancelFunc
	root     *Controller
	releases []func()
	done     chan struct{}
}

// GetHTTPPathPrefix returns the projected content route for this live runtime.
func (r *appRuntime) GetHTTPPathPrefix() string { return r.root.httpPathPrefix }

// InvokeMethod scopes RPC cancellation and plugin storage to this installation.
func (r *appRuntime) InvokeMethod(service, method string, stream srpc.Stream) (bool, error) {
	ctx, cancel := context.WithCancel(stream.Context())
	stop := context.AfterFunc(r.ctx, cancel)
	defer stop()
	defer cancel()
	ctx = storage.WithHostStorageID(ctx, "default")
	ctx = bldr_plugin.WithPluginContextInfo(ctx, nil)
	return r.root.InvokeMethod(service, method, srpc.NewStreamWithContext(stream, ctx))
}

// close releases controllers before dropping the private World, if one was created.
func (r *appRuntime) close() {
	defer close(r.done)
	r.cancel()
	for _, release := range slices.Backward(r.releases) {
		release()
	}
	r.releases = nil
}

// buildApp composes the normal local provider, Session, Space, plugin, and Root
// controllers around supplied Storage. Its bus never inherits parent Sessions.
func (c *Controller) buildApp(ctx context.Context, binding resource_root.AppStorage) (_ *appRuntime, err error) {
	ctx, cancel := context.WithCancel(bldr_plugin.WithPluginContextInfo(ctx, nil))
	ctx = storage.WithHostStorageID(ctx, "default")
	runtime := &appRuntime{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	defer func() {
		if err != nil {
			runtime.close()
		}
	}()
	le := c.GetLogger().WithField("runtime", "nested-app")
	child, factories, err := bldr_core.NewCoreBus(ctx, le)
	if err != nil {
		return nil, err
	}
	db_core.AddFactories(child, factories)
	for _, factory := range []controller.Factory{
		NewFactory(child, withParent(c)),
		provider_local.NewFactory(child),
		space_http_download.NewFactory(child),
		space_http_export.NewFactory(child),
		session_controller.NewFactory(child),
		space_sobject.NewFactory(child),
		sobject_world_engine.NewFactory(child),
		space_world_optypes.NewFactory(child),
		space_world_blocktype.NewFactory(child),
		plugin_space.NewFactory(child, plugin_space.WithManifestSource(c.GetBus())),
		plugin_host_scheduler.NewFactory(child),
		plugin_host_configset.NewFactory(child),
		plugin_host_storage_volume.NewFactory(child),
		world_block_engine.NewFactory(child),
		volume_rpc_server.NewFactory(child),
		object_peer.NewFactory(child),
	} {
		factories.AddFactory(factory)
	}
	start := func(conf config.Config) (controller.Controller, error) {
		ctrl, _, ref, err := loader.WaitExecControllerRunning(ctx, child, resolver.NewLoadControllerWithConfig(conf), nil)
		if err == nil {
			runtime.releases = append(runtime.releases, ref.Release)
		}
		return ctrl, err
	}
	add := func(ctrl controller.Controller) error {
		release, err := child.AddController(ctx, ctrl, nil)
		if err == nil {
			runtime.releases = append(runtime.releases, release)
		}
		return err
	}
	if _, err := start(&node_controller.Config{}); err != nil {
		return nil, err
	}

	// Inherit read-only manifest lookup and host execution capabilities. Space
	// plugins load on the parent's plugin host and register with the parent's
	// shared registries. Mutable storage, Sessions, and resource services
	// resolve exclusively on the child.
	bridge := bus_bridge.NewBusBridge(c.GetBus(), func(di directive.Instance) (bool, error) {
		switch di.GetDirective().(type) {
		case bldr_manifest.FetchManifest, bldr_plugin.LoadPlugin, plugin_host.LookupPluginHost, plugin_host_root.LookupRoot:
			return true, nil
		default:
			return false, nil
		}
	})
	if err := add(bridge); err != nil {
		return nil, err
	}

	var supplied storage.Storage
	if binding.StorageID != "" {
		selected, _, ref, lookupErr := bus.ExecWaitValue[storage.LookupStorageValue](
			ctx, c.GetBus(), storage.NewLookupStorage(binding.StorageID), bus.ReturnIfIdle(true), cancel, nil,
		)
		if lookupErr != nil {
			return nil, lookupErr
		}
		runtime.releases = append(runtime.releases, ref.Release)
		supplied = selected
	} else {
		if binding.Engine != nil {
			if err := add(&appWorldController{engine: binding.Engine}); err != nil {
				return nil, err
			}
		} else {
			backingCtrl, err := start(&volume_kvtxinmem.Config{VolumeConfig: &volume_controller.Config{
				VolumeIdAlias: []string{"app-backing-volume"}, DisablePeer: true,
			}})
			if err != nil {
				return nil, err
			}
			backing, err := backingCtrl.(volume.Controller).GetVolume(ctx)
			if err != nil {
				return nil, err
			}
			bucketID := "app-backing-bucket"
			if _, _, _, err := backing.ApplyBucketConfig(ctx, &bucket.Config{Id: bucketID, Rev: 1}); err != nil {
				return nil, err
			}
			if _, err := start(world_block_engine.NewConfig(appBackingEngineID, backing.GetID(), bucketID,
				"app-backing-head", &bucket.ObjectRef{BucketId: bucketID}, nil, true)); err != nil {
				return nil, err
			}
		}
		prefix := binding.Prefix
		if prefix == "" {
			prefix = "app"
		}
		supplied, err = storage_world.NewStorage(ctx, child, appBackingEngineID, prefix)
		if err != nil {
			return nil, err
		}
	}
	supplied.AddFactories(child, factories)
	if err := add(storage_controller.BuildStorageController("default", []storage.Storage{supplied},
		controller.NewInfo("app/storage", Version, "app storage"))); err != nil {
		return nil, err
	}

	// The installation owns the plugin host Volume and each provider owns its
	// usual separately named account Volumes through the selected Storage.
	pluginVolumeCtrl, err := start(&storage_volume.Config{
		StorageId: "default", StorageVolumeId: "plugin-host",
		VolumeConfig: &volume_controller.Config{VolumeIdAlias: []string{bldr_plugin.PluginVolumeID}},
	})
	if err != nil {
		return nil, err
	}
	if _, err := pluginVolumeCtrl.(volume.Controller).GetVolume(ctx); err != nil {
		return nil, err
	}
	for _, conf := range []config.Config{
		&object_peer.Config{ObjectStoreId: "s4wave-peer", VolumeId: bldr_plugin.PluginVolumeID},
		&session_controller.Config{},
		&provider_local.Config{StorageId: "default"},
		&space_sobject.Config{},
		&space_world_ops.Config{},
		&space_world_blocktype.Config{},
		&space_http_download.Config{},
		&space_http_export.Config{},
	} {
		if _, err := start(conf); err != nil {
			return nil, err
		}
	}
	root, err := start(&Config{})
	if err != nil {
		return nil, err
	}
	runtime.root = root.(*Controller)
	pathPrefix := "/app/" + ulid.NewULID()
	runtime.root.httpPathPrefix = c.httpPathPrefix + pathPrefix
	handler := http.StripPrefix(pathPrefix, bifrost_http.NewBusHandler(child, "", true))
	httpCtrl := bifrost_http.NewHTTPHandlerController(
		controller.NewInfo("app/http", Version, "projected app files and exports"),
		func(context.Context, func()) (http.Handler, func(), error) {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				requestCtx, requestCancel := context.WithCancel(req.Context())
				stop := context.AfterFunc(ctx, requestCancel)
				defer stop()
				defer requestCancel()
				requestCtx = storage.WithHostStorageID(requestCtx, "default")
				requestCtx = bldr_plugin.WithPluginContextInfo(requestCtx, nil)
				handler.ServeHTTP(w, req.WithContext(requestCtx))
			}), nil, nil
		}, []string{pathPrefix + "/"}, false, nil)
	release, err := c.GetBus().AddController(ctx, httpCtrl, nil)
	if err != nil {
		return nil, err
	}
	runtime.releases = append(runtime.releases, release)
	return runtime, nil
}

// appWorldController exposes only the World capability selected by the caller.
type appWorldController struct{ engine world.Engine }

func (c *appWorldController) GetControllerInfo() *controller.Info {
	return controller.NewInfo("app/world", Version, "app backing World")
}
func (c *appWorldController) Execute(context.Context) error { return nil }
func (c *appWorldController) Close() error                  { return nil }
func (c *appWorldController) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	if lookup, ok := di.GetDirective().(world.LookupWorldEngine); ok && lookup.LookupWorldEngineID() == appBackingEngineID {
		return directive.R(directive.NewValueResolver([]world.Engine{c.engine}), nil)
	}
	return nil, nil
}
