//go:build js
// +build js

package main

import (
	"context"
	"io"
	"net/http"
	"syscall/js"

	link_establish_controller "github.com/aperturerobotics/bifrost/link/establish"
	stream_srpc_client_controller "github.com/aperturerobotics/bifrost/stream/srpc/client/controller"
	"github.com/aperturerobotics/bifrost/transport/websocket"
	"github.com/aperturerobotics/bldr/banner"
	"github.com/aperturerobotics/bldr/core"
	devtool_web "github.com/aperturerobotics/bldr/devtool/web"
	devtool_web_entrypoint_controller "github.com/aperturerobotics/bldr/devtool/web/entrypoint/controller"
	manifest_fetch_rpc "github.com/aperturerobotics/bldr/manifest/fetch/rpc"
	plugin_host_web "github.com/aperturerobotics/bldr/plugin/host/web"
	web_entrypoint_browser "github.com/aperturerobotics/bldr/web/entrypoint/browser"
	bldr_web_plugin_browser_controller "github.com/aperturerobotics/bldr/web/plugin/browser/controller"
	lookup_concurrent "github.com/aperturerobotics/hydra/bucket/lookup/concurrent"
	node_controller "github.com/aperturerobotics/hydra/node/controller"
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
		sr.AddFactory(web_entrypoint_browser.NewFactory(b))
		sr.AddFactory(plugin_host_web.NewFactory(b))
		sr.AddFactory(websocket.NewFactory(b))
		sr.AddFactory(link_establish_controller.NewFactory(b))
		sr.AddFactory(manifest_fetch_rpc.NewFactory(b))
		sr.AddFactory(stream_srpc_client_controller.NewFactory(b))
		sr.AddFactory(lookup_concurrent.NewFactory(b))
		sr.AddFactory(bldr_web_plugin_browser_controller.NewFactory(b))

		nodeCtrl := node_controller.NewController(&node_controller.Config{}, le, b)
		relNodeCtrl, err := b.AddController(ctx, nodeCtrl, nil)
		if err != nil {
			return err
		}
		defer relNodeCtrl()

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
