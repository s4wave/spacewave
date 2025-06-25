//go:build js
// +build js

package devtool_web_entrypoint_controller

import (
	"context"

	link_establish_controller "github.com/aperturerobotics/bifrost/link/establish"
	stream_srpc_client "github.com/aperturerobotics/bifrost/stream/srpc/client"
	stream_srpc_client_controller "github.com/aperturerobotics/bifrost/stream/srpc/client/controller"
	"github.com/aperturerobotics/bifrost/transport/common/dialer"
	"github.com/aperturerobotics/bifrost/transport/websocket"
	devtool_web "github.com/aperturerobotics/bldr/devtool/web"
	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	manifest_fetch_rpc "github.com/aperturerobotics/bldr/manifest/fetch/rpc"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_plugin "github.com/aperturerobotics/bldr/plugin"
	plugin_host_scheduler "github.com/aperturerobotics/bldr/plugin/host/scheduler"
	plugin_host_web "github.com/aperturerobotics/bldr/plugin/host/web"
	storage_default "github.com/aperturerobotics/bldr/storage/default"
	storage_volume "github.com/aperturerobotics/bldr/storage/volume"
	browser "github.com/aperturerobotics/bldr/web/entrypoint/browser"
	bldr_web_plugin_browser_controller "github.com/aperturerobotics/bldr/web/plugin/browser/controller"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/bucket"
	volume_controller "github.com/aperturerobotics/hydra/volume/controller"
	volume_rpc_client "github.com/aperturerobotics/hydra/volume/rpc/client"
	"github.com/aperturerobotics/hydra/world"
	world_block_engine "github.com/aperturerobotics/hydra/world/block/engine"
	"github.com/aperturerobotics/util/backoff"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
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
}

func NewController(
	le *logrus.Entry,
	b bus.Bus,
	devtoolInfo *devtool_web.DevtoolInitBrowser,
	initm *web_runtime.WebRuntimeHostInit,
	linkUrl string,
) *Controller {
	return &Controller{
		le:          le,
		b:           b,
		devtoolInfo: devtoolInfo,
		initm:       initm,
		linkUrl:     linkUrl,
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

	// run the browser web runtime controller
	webRuntimeID := c.initm.GetWebRuntimeId()
	_, _, rtRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&browser.Config{
			WebRuntimeId: webRuntimeID,
			MessagePort:  "BLDR_WEB_RUNTIME_CLIENT_OPEN",
		}),
		nil,
	)
	if err != nil {
		err = errors.Wrap(err, "start runtime controller")
		return err
	}
	defer rtRef.Release()

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

	// load the web plugin browser host controller
	// services any web plugins forwarding their request to the plugin host
	// starts the web plugin controller
	_, _, webPluginBrowserHostRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&bldr_web_plugin_browser_controller.Config{}),
		nil,
	)
	if err != nil {
		err = errors.Wrap(err, "start web plugin browser host controller")
		return err
	}
	defer webPluginBrowserHostRef.Release()

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
		// we want to store manifests in the world state
		DisableStoreManifest: false,
	})
	pluginSchecCtrlRel, err := b.AddController(ctx, pluginSchedCtrl, func(err error) {
		le.WithError(err).Error("plugin scheduler controller failed")
	})
	if err != nil {
		return err
	}
	defer pluginSchecCtrlRel()

	// run the web browser plugin loader implementation
	webPluginHostCtrl, webPluginHost, err := plugin_host_web.NewWebHostController(le, b, &plugin_host_web.Config{WebRuntimeId: webRuntimeID})
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

	// Call LoadPlugin for the list of Start plugins.
	for _, pluginID := range devtoolInfo.GetStartPlugins() {
		le.WithField("plugin-id", pluginID).Info("loading startup plugin")
		_, plugRef, err := b.AddDirective(bldr_plugin.NewLoadPlugin(pluginID), nil)
		if err != nil {
			return err
		}
		defer plugRef.Release()
	}

	// wait to run all the defer calls until context cancels
	<-ctx.Done()
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The passed context is canceled when the directive instance expires.
// NOTE: the passed context is not canceled when the handler is removed.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
