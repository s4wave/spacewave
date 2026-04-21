//go:build js

package devtool_web_entrypoint_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/aperturerobotics/util/backoff"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	devtool_web "github.com/s4wave/spacewave/bldr/devtool/web"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	manifest_fetch_rpc "github.com/s4wave/spacewave/bldr/manifest/fetch/rpc"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	plugin_host_scheduler "github.com/s4wave/spacewave/bldr/plugin/host/scheduler"
	plugin_host_web "github.com/s4wave/spacewave/bldr/plugin/host/web"
	storage_default "github.com/s4wave/spacewave/bldr/storage/default"
	storage_volume "github.com/s4wave/spacewave/bldr/storage/volume"
	web_runtime "github.com/s4wave/spacewave/bldr/web/runtime"
	web_runtime_bootstrap "github.com/s4wave/spacewave/bldr/web/runtime/bootstrap"
	"github.com/s4wave/spacewave/db/bucket"
	volume_controller "github.com/s4wave/spacewave/db/volume/controller"
	volume_rpc_client "github.com/s4wave/spacewave/db/volume/rpc/client"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/s4wave/spacewave/net/link"
	link_establish_controller "github.com/s4wave/spacewave/net/link/establish"
	bifrost_rpc "github.com/s4wave/spacewave/net/rpc"
	stream_srpc_client "github.com/s4wave/spacewave/net/stream/srpc/client"
	stream_srpc_client_controller "github.com/s4wave/spacewave/net/stream/srpc/client/controller"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller ID.
const ControllerID = "bldr/devtool/web/entrypoint"

// Version is the version of this controller.
var Version = semver.MustParse("0.0.1")

// Controller manages the devtool web entrypoint.
type Controller struct {
	le *logrus.Entry
	b  bus.Bus

	devtoolInfo *devtool_web.DevtoolInitBrowser
	initm       *web_runtime.WebRuntimeHostInit
	linkUrl     string

	// browserRpcServer handles incoming SRPC streams on BrowserProtocolID.
	// Initialized in the constructor so it is ready before Execute runs.
	browserRpcServer *srpc.Server
}

func NewController(
	le *logrus.Entry,
	b bus.Bus,
	devtoolInfo *devtool_web.DevtoolInitBrowser,
	initm *web_runtime.WebRuntimeHostInit,
	linkUrl string,
) *Controller {
	// Set up the browser RPC server immediately so HandleDirective can
	// accept incoming BrowserProtocolID streams as soon as the controller
	// is added to the bus. The mux falls back to bifrost_rpc.NewInvoker
	// which lazily resolves any RPC service on the browser bus (including
	// plugin-provided services once they load).
	browserMux := srpc.NewMux(bifrost_rpc.NewInvoker(b, devtool_web.BrowserProtocolID.String(), true))
	return &Controller{
		le:               le,
		b:                b,
		devtoolInfo:      devtoolInfo,
		initm:            initm,
		linkUrl:          linkUrl,
		browserRpcServer: srpc.NewServer(browserMux),
	}
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "devtool web entrypoint")
}

// Execute executes the controller.
// Returning nil ends execution.
// NOTE: we le.Fatal a lot of things in here
func (c *Controller) Execute(ctx context.Context) (rerr error) {
	b, le, devtoolInfo := c.b, c.le, c.devtoolInfo

	runtimeStack, err := web_runtime_bootstrap.StartRuntimeStack(
		ctx,
		le,
		b,
		web_runtime_bootstrap.RuntimeStackOpts{
			WebRuntimeID: c.initm.GetWebRuntimeId(),
			MessagePort:  "BLDR_WEB_RUNTIME_CLIENT_OPEN",
		},
	)
	if err != nil {
		return errors.Wrap(err, "start runtime stack")
	}
	defer runtimeStack.Release()

	// run the dist storage
	storageID := storage_default.StorageID
	storageVolCtrl, volCtrlRef, err := storage_volume.ExecVolumeController(ctx, b, &storage_volume.Config{
		StorageId:       storageID,
		StorageVolumeId: "devtool/dist/" + devtoolInfo.GetAppId(),
		VolumeConfig: &volume_controller.Config{
			VolumeIdAlias: []string{"dist"},
		},
	})
	if err != nil {
		return err
	}
	defer volCtrlRef.Release()

	storageVol, err := storageVolCtrl.GetVolume(ctx)
	if err != nil {
		return err
	}

	// connect to the devtool via. WebSocket so we can fetch manifests
	devtoolBackoff := &backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			MaxElapsedTime: 2400,
		},
	}
	_, _, wsRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&websocket.Config{
		Dialers: map[string]*dialer.DialerOpts{
			devtoolInfo.GetDevtoolPeerId(): {
				Address: c.linkUrl,
				Backoff: devtoolBackoff,
			},
		},
	}), nil)
	if err != nil {
		err = errors.Wrap(err, "start websocket controller")
		return err
	}
	defer wsRef.Release()

	// run the link establish controller to keep a connection with the devtool
	_, _, wsEstRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&link_establish_controller.Config{
		PeerIds: []string{devtoolInfo.GetDevtoolPeerId()},
	}), nil)
	if err != nil {
		err = errors.Wrap(err, "start websocket controller")
		return err
	}
	defer wsEstRef.Release()

	// forward RPC service ids with the HostServiceID to the devtool
	// this will forward LookupRpcClient<devtool/*>
	fwdDevtoolCtrlI, _, fwdDevtoolRpcRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&stream_srpc_client_controller.Config{
		Client: &stream_srpc_client.Config{
			ServerPeerIds:    []string{devtoolInfo.GetDevtoolPeerId()},
			PerServerBackoff: devtoolBackoff,
			TimeoutDur:       "4s",
		},
		ServiceIdPrefixes: []string{devtool_web.HostServiceIDPrefix},
		ProtocolId:        devtool_web.HostProtocolID.String(),
	}), nil)
	if err != nil {
		err = errors.Wrap(err, "start fetch manifest via rpc controller")
		return err
	}
	defer fwdDevtoolRpcRef.Release()

	// get the srpc.Client for the devtool
	fwdDevtoolCtrl := fwdDevtoolCtrlI.(*stream_srpc_client_controller.Controller)
	devtoolPrefixClient, devtoolBaseClient := fwdDevtoolCtrl.GetClient(), fwdDevtoolCtrl.GetBaseClient()
	_ = devtoolPrefixClient

	// forward LookupVolume directives via RPC to the devtool
	devtoolVolumeInfo := devtoolInfo.GetDevtoolVolumeInfo()
	devtoolVolumeID := devtool_web.HostVolumeID
	devtoolVolumeController := volume_rpc_client.NewProxyVolumeControllerWithClient(
		b,
		le,
		devtoolVolumeInfo,
		[]string{devtoolVolumeID},
		devtoolBaseClient,
		devtool_web.HostVolumeServiceIDPrefix,
	)
	relDevtoolVolumeController, err := b.AddController(ctx, devtoolVolumeController, func(err error) {
		le.WithError(err).Error("devtool volume proxy controller failed")
	})
	if err != nil {
		return err
	}
	defer relDevtoolVolumeController()

	// forward FetchManifest directives via RPC to the devtool
	_, _, fwdFmRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&manifest_fetch_rpc.Config{
		ServiceId: devtool_web.HostServiceIDPrefix + bldr_manifest.SRPCManifestFetchServiceID,
		ClientId:  devtool_web.EntrypointClientID,
	}), nil)
	if err != nil {
		err = errors.Wrap(err, "start fetch manifest via rpc controller")
		return err
	}
	defer fwdFmRef.Release()

	pluginBrowserHostRel, err := web_runtime_bootstrap.StartPluginBrowserHost(ctx, b, nil)
	if err != nil {
		return err
	}
	defer pluginBrowserHostRel()

	// run a hydra world for storing plugin host state and manifests
	engineID := "bldr/dev-plugin-host"
	engineBucketID := engineID
	engineObjStoreID := engineBucketID
	pluginHostObjKey := "dev-plugin-host"
	engineVolumeID := devtoolVolumeID

	// create state bucket if it doesn't exist
	engineBucketConf, err := bucket.NewConfig(engineBucketID, 1, nil, nil)
	if err != nil {
		return err
	}
	_, err = bucket.ExApplyBucketConfig(ctx, b, bucket.NewApplyBucketConfigToVolume(engineBucketConf, engineVolumeID))
	if err != nil {
		return errors.Wrap(err, "apply bucket config")
	}

	// start the block engine
	engConf := world_block_engine.NewConfig(
		engineID,
		engineVolumeID,
		engineBucketID,
		engineObjStoreID,
		&bucket.ObjectRef{BucketId: engineBucketID},
		nil,
		false,
	)
	worldCtrl, worldCtrlRef, err := world_block_engine.StartEngineWithConfig(
		ctx,
		b,
		engConf,
	)
	if err != nil {
		err = errors.Wrap(err, "start world controller")
		return err
	}
	defer worldCtrlRef.Release()

	eng, err := worldCtrl.GetWorldEngine(ctx)
	if err != nil {
		return err
	}
	// worldState := world.NewEngineWorldState(eng, true)

	// register the world operation types for plugin host
	lookupOpCtrl := world.NewLookupOpController("bldr-plugin-host-ops", engineID, bldr_manifest_world.LookupOp)
	relLookupCtrl, err := b.AddController(ctx, lookupOpCtrl, nil)
	if err != nil {
		return err
	}
	defer relLookupCtrl()

	// ensure the plugin host exists in the world
	engTx, err := eng.NewTransaction(ctx, true)
	if err != nil {
		return err
	}

	_, err = bldr_manifest_world.CreateManifestStore(ctx, engTx, pluginHostObjKey)
	if err != nil {
		engTx.Discard()
		return err
	}

	if err := engTx.Commit(ctx); err != nil {
		engTx.Discard()
		return err
	}

	// run the plugin scheduler
	pluginSchedCtrl := plugin_host_scheduler.NewController(le, b, &plugin_host_scheduler.Config{
		EngineId:  engineID,
		ObjectKey: pluginHostObjKey,
		PeerId:    storageVol.GetPeerID().String(),
		VolumeId:  storageVol.GetID(),

		// we want FetchManifest directives
		WatchFetchManifest: true,
		// we want to use the devtool volume (via websocket) to load assets
		// no need to copy into the browser storage in devtool mode
		DisableCopyManifest: true,
		// fetched manifest refs point at the devtool bucket, so browser-side
		// hosts should execute them directly instead of storing those refs into
		// the local plugin-host world and later failing to dereference them.
		DisableStoreManifest: true,
	})
	pluginSchecCtrlRel, err := b.AddController(ctx, pluginSchedCtrl, func(err error) {
		le.WithError(err).Error("plugin scheduler controller failed")
	})
	if err != nil {
		return err
	}
	defer pluginSchecCtrlRel()

	// run the web browser plugin loader implementation (for "web/js/wasm" platform)
	webPluginHostCtrl, webPluginHost, err := plugin_host_web.NewWebHostController(le, b, &plugin_host_web.Config{
		WebRuntimeId:          c.initm.GetWebRuntimeId(),
		ForceDedicatedWorkers: devtoolInfo.GetForceDedicatedWorkers(),
	})
	if err != nil {
		err = errors.Wrap(err, "start web host controller")
		return err
	}
	webPluginHostRel, err := b.AddController(ctx, webPluginHostCtrl, func(err error) {
		le.WithError(err).Error("plugin host controller failed")
	})
	if err != nil {
		err = errors.Wrap(err, "start web plugin host")
		return err
	}
	defer webPluginHostRel()
	le.Info("web plugin host is running")
	_ = webPluginHost

	// run the QuickJS web browser plugin host (for "js" platform)
	webQuickJSHostCtrl, webQuickJSHost, err := plugin_host_web.NewWebQuickJSHostController(le, b, &plugin_host_web.QuickJSConfig{
		WebRuntimeId:          c.initm.GetWebRuntimeId(),
		ForceDedicatedWorkers: devtoolInfo.GetForceDedicatedWorkers(),
	})
	if err != nil {
		err = errors.Wrap(err, "start web quickjs host controller")
		return err
	}
	webQuickJSHostRel, err := b.AddController(ctx, webQuickJSHostCtrl, func(err error) {
		le.WithError(err).Error("quickjs plugin host controller failed")
	})
	if err != nil {
		err = errors.Wrap(err, "start web quickjs plugin host")
		return err
	}
	defer webQuickJSHostRel()
	le.Info("web quickjs plugin host is running")
	_ = webQuickJSHost

	// Call LoadPlugin for the list of Start plugins.
	for _, pluginID := range devtoolInfo.GetStartPlugins() {
		le.WithField("plugin-id", pluginID).Info("loading startup plugin")
		_, plugRef, err := b.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
		if err != nil {
			return err
		}
		defer plugRef.Release()
	}

	le.Info("browser RPC server ready for BrowserProtocolID streams")

	// wait to run all the defer calls until context cancels
	<-ctx.Done()
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case link.HandleMountedStream:
		return c.resolveHandleMountedStream(ctx, di, d)
	}
	return nil, nil
}

// resolveHandleMountedStream handles incoming streams on BrowserProtocolID.
func (c *Controller) resolveHandleMountedStream(
	_ context.Context,
	_ directive.Instance,
	dir link.HandleMountedStream,
) ([]directive.Resolver, error) {
	if dir.HandleMountedStreamProtocolID() != devtool_web.BrowserProtocolID {
		return nil, nil
	}
	return directive.Resolvers(directive.NewValueResolver([]link.MountedStreamHandler{c})), nil
}

// HandleMountedStream handles an incoming mounted stream on BrowserProtocolID.
func (c *Controller) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	strm := ms.GetStream()
	sctx := link.WithMountedStreamContext(ctx, ms)
	go func() {
		c.browserRpcServer.HandleStream(sctx, strm)
		strm.Close()
	}()
	return nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ controller.Controller     = ((*Controller)(nil))
	_ link.MountedStreamHandler = ((*Controller)(nil))
)
