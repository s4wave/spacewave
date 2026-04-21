//go:build js
// +build js

package main

import (
	"context"
	"io"
	"net/http"
	"syscall/js"

	link_establish_controller "github.com/s4wave/spacewave/net/link/establish"
	link_holdopen_controller "github.com/s4wave/spacewave/net/link/hold-open"
	stream_srpc_client_controller "github.com/s4wave/spacewave/net/stream/srpc/client/controller"
	"github.com/s4wave/spacewave/net/transport/websocket"
	"github.com/s4wave/spacewave/bldr/banner"
	"github.com/s4wave/spacewave/bldr/core"
	devtool_web "github.com/s4wave/spacewave/bldr/devtool/web"
	devtool_web_entrypoint_controller "github.com/s4wave/spacewave/bldr/devtool/web/entrypoint/controller"
	manifest_fetch_rpc "github.com/s4wave/spacewave/bldr/manifest/fetch/rpc"
	plugin_host_web "github.com/s4wave/spacewave/bldr/plugin/host/web"
	default_storage "github.com/s4wave/spacewave/bldr/storage/default"
	web_entrypoint_browser "github.com/s4wave/spacewave/bldr/web/entrypoint/browser"
	bldr_web_plugin_browser_controller "github.com/s4wave/spacewave/bldr/web/plugin/browser/controller"
	configset_controller "github.com/aperturerobotics/controllerbus/controller/configset/controller"
	bucket_setup "github.com/s4wave/spacewave/db/bucket/setup"
	node_controller "github.com/s4wave/spacewave/db/node/controller"
	volume_rpc_server "github.com/s4wave/spacewave/db/volume/rpc/server"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/retry"
	"github.com/sirupsen/logrus"
)

// LogLevel is the default log level to use.
var LogLevel = logrus.DebugLevel

func main() {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(LogLevel)
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	le := logrus.NewEntry(log)

	// get the init message from the bldr js runtime
	initm, err := web_entrypoint_browser.ReadInitMessage()
	if err != nil {
		le.WithError(err).Fatal("failed to read init message")
	}
	banner.WriteToConsole()

	// get the urls to the devtool server
	locationOrigin := getLocationOrigin()
	infoUrl := locationOrigin + "/bldr-dev/web-wasm/info"
	linkUrl := locationOrigin + "/bldr-dev/web-wasm/link.ws"

	// backoff
	devtoolBackoff := &backoff.Backoff{
		BackoffKind: backoff.BackoffKind_BackoffKind_EXPONENTIAL,
		Exponential: &backoff.Exponential{
			MaxElapsedTime: 2400,
		},
	}

	// get the info from the devtool info endpoint
	err = retry.Retry(ctx, le, func(ctx context.Context, success func()) error {
		resp, err := http.Get(infoUrl)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		devtoolInfo := &devtool_web.DevtoolInitBrowser{}
		if err := devtoolInfo.UnmarshalVT(data); err != nil {
			return err
		}
		success()

		b, sr, err := core.NewCoreBus(ctx, le)
		if err != nil {
			le.Fatal(err.Error())
		}

		sr.AddFactory(manifest_fetch_rpc.NewFactory(b))
		sr.AddFactory(plugin_host_web.NewFactory(b))
		sr.AddFactory(web_entrypoint_browser.NewFactory(b))
		sr.AddFactory(bldr_web_plugin_browser_controller.NewFactory(b))
		sr.AddFactory(world_block_engine.NewFactory(b))

		sr.AddFactory(stream_srpc_client_controller.NewFactory(b))
		sr.AddFactory(volume_rpc_server.NewFactory(b))

		sr.AddFactory(websocket.NewFactory(b))
		sr.AddFactory(link_establish_controller.NewFactory(b))
		sr.AddFactory(link_holdopen_controller.NewFactory(b))

		sr.AddFactory(bucket_setup.NewFactory(b))

		nodeCtrl := node_controller.NewController(&node_controller.Config{}, le, b)
		relNodeCtrl, err := b.AddController(ctx, nodeCtrl, nil)
		if err != nil {
			return err
		}
		defer relNodeCtrl()

		// attach the configset controller
		configSetCtrl, _ := configset_controller.NewController(le, b)
		relConfigSetCtrl, err := b.AddController(ctx, configSetCtrl, nil)
		if err != nil {
			return err
		}
		defer relConfigSetCtrl()

		// attach the default storage controller
		storageID := default_storage.StorageID
		storageCtrl := default_storage.NewController(storageID, b, "")
		relStorageCtrl, err := b.AddController(ctx, storageCtrl, nil)
		if err != nil {
			return err
		}
		defer relStorageCtrl()

		// add storage factories
		for _, st := range storageCtrl.GetStorage() {
			st.AddFactories(b, sr)
		}

		ctrl := devtool_web_entrypoint_controller.NewController(
			le,
			b,
			devtoolInfo,
			initm,
			linkUrl,
		)
		defer b.RemoveController(ctrl)
		return b.ExecuteController(ctx, ctrl)
	}, retry.NewBackOff(devtoolBackoff))
	if err != nil {
		le.WithError(err).Fatal("failed to execute devtool controller")
	}
}

// getLocationOrigin returns the worker location URL.
func getLocationOrigin() string {
	global := js.Global()
	location := global.Get("location")
	origin := location.Get("origin")
	return origin.String()
}
