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
	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	plugin_host_web "github.com/aperturerobotics/bldr/plugin/host/web"
	"github.com/aperturerobotics/bldr/storage"
	browser_storage "github.com/aperturerobotics/bldr/storage/browser"
	"github.com/aperturerobotics/bldr/web/plugin/browser"
	web_runtime "github.com/aperturerobotics/bldr/web/runtime"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/backoff"
	"github.com/blang/semver"
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
func (c *Controller) Execute(ctx context.Context) (rerr error) {
	// run the browser storage
	b, le, devtoolInfo := c.b, c.le, c.devtoolInfo

	browserStorage := browser_storage.BuildStorage(b, "")
	storageRel := storage.ExecuteStorage(ctx, b, le, browserStorage, devtoolInfo.GetAppId())
	defer storageRel()

	// run the browser web runtime controller
	_, _, rtRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&browser.Config{
			WebRuntimeId: c.initm.GetWebRuntimeId(),
			MessagePort:  "BLDR_WEB_RUNTIME_CLIENT_OPEN",
		}),
		nil,
	)
	if err != nil {
		err = errors.Wrap(err, "start runtime controller")
		le.Fatal(err.Error())
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
		le.Fatal(err.Error())
	}
	defer wsRef.Release()

	// run the link establish controller to keep a connection with the devtool
	_, _, wsEstRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&link_establish_controller.Config{
		PeerIds: []string{devtoolInfo.GetDevtoolPeerId()},
	}), nil)
	if err != nil {
		err = errors.Wrap(err, "start websocket controller")
		le.Fatal(err.Error())
	}
	defer wsEstRef.Release()

	// forward RPC service ids with the HostServiceID to the devtool
	// this will forward LookupRpcClient<devtool/*>
	_, _, fwdDevtoolRpcRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&stream_srpc_client_controller.Config{
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
		le.Fatal(err.Error())
	}
	defer fwdDevtoolRpcRef.Release()

	// forward FetchManifest directives via RPC to the devtool
	_, _, fwdFmRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&manifest_fetch_rpc.Config{
		ServiceId: devtool_web.HostServiceIDPrefix + bldr_manifest.SRPCManifestFetchServiceID,
		ClientId:  devtool_web.EntrypointClientID,
	}), nil)
	if err != nil {
		err = errors.Wrap(err, "start fetch manifest via rpc controller")
		le.Fatal(err.Error())
	}
	defer fwdFmRef.Release()

	// run the browser plugin host controller
	_, _, phRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&plugin_host_web.Config{
			HostConfig:   &plugin_host_controller.Config{},
			WebRuntimeId: c.initm.GetWebRuntimeId(),
		}),
		nil,
	)
	if err != nil {
		err = errors.Wrap(err, "start web plugin host")
		// le.Fatal(err.Error())
		le.Error(err.Error())
	}
	if phRef != nil {
		defer phRef.Release()
	}

	// TODO
	/*
		demoManifest, err := bldr_manifest.ExFetchManifest(ctx, b, &bldr_manifest.ManifestMeta{
			ManifestId: "bldr-demo",
			PlatformId: "web",
		}, false)
		if err != nil {
			le.Fatal(err.Error())
		}
		le.Infof("got demo manifest from devtool: %v", demoManifest.String())
	*/
	_, fetchRef, err := b.AddDirective(bldr_manifest.NewFetchManifest(&bldr_manifest.ManifestMeta{
		ManifestId: "bldr-demo",
		PlatformId: "web",
	}), bus.NewCallbackHandler(func(v directive.AttachedValue) {
		demoManifest := v.GetValue().(*bldr_manifest.FetchManifestValue)
		le.Infof("got demo manifest from devtool: %v", demoManifest.String())

	}, nil, nil))
	if err != nil {
		le.Error(err.Error())
	}
	defer fetchRef.Release()

	<-ctx.Done()
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns resolver(s). If not, returns nil.
// It is safe to add a reference to the directive during this call.
// The passed context is canceled when the directive instance expires.
// NOTE: the passed context is not canceled when the handler is removed.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	// TODO
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	// TODO
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))
