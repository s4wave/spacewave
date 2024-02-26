//go:build js
// +build js

package main

import (
	"context"
	"io"
	"net/http"
	"syscall/js"

	link_establish_controller "github.com/aperturerobotics/bifrost/link/establish"
	"github.com/aperturerobotics/bifrost/transport/common/dialer"
	"github.com/aperturerobotics/bifrost/transport/websocket"
	"github.com/aperturerobotics/bldr/banner"
	"github.com/aperturerobotics/bldr/core"
	devtool_web "github.com/aperturerobotics/bldr/devtool/web"
	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	plugin_host_web "github.com/aperturerobotics/bldr/plugin/host/web"
	"github.com/aperturerobotics/bldr/storage"
	browser_storage "github.com/aperturerobotics/bldr/storage/browser"
	web_entrypoint_browser "github.com/aperturerobotics/bldr/web/entrypoint/browser"
	"github.com/aperturerobotics/bldr/web/plugin/browser"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/aperturerobotics/util/backoff"
	"github.com/aperturerobotics/util/retry"
	"github.com/pkg/errors"
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
	writeBanner()

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
	var devtoolInfo *devtool_web.DevtoolInitBrowser
	err = retry.Retry(ctx, le, func(ctx context.Context, success func()) error {
		resp, err := http.Get(infoUrl)
		if err != nil {
			return err
		}
		data, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		out := &devtool_web.DevtoolInitBrowser{}
		if err := out.UnmarshalVT(data); err != nil {
			return err
		}
		devtoolInfo = out
		success()
		return nil
	}, retry.NewBackOff(devtoolBackoff))
	if err != nil {
		le.WithError(err).Fatal("failed to read init message")
	}

	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		le.Fatal(err.Error())
	}
	sr.AddFactory(browser.NewFactory(b))
	sr.AddFactory(plugin_host_web.NewFactory(b))
	sr.AddFactory(websocket.NewFactory(b))
	sr.AddFactory(link_establish_controller.NewFactory(b))

	// run the browser storage
	browserStorage := browser_storage.BuildStorage(b, "")
	storageRel := storage.ExecuteStorage(ctx, b, le, browserStorage, devtoolInfo.GetAppId())
	defer storageRel()

	// run the browser web runtime controller
	_, _, rtRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&browser.Config{
			WebRuntimeId: initm.GetWebRuntimeId(),
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
	_, _, wsRef, err := loader.WaitExecControllerRunning(ctx, b, resolver.NewLoadControllerWithConfig(&websocket.Config{
		Dialers: map[string]*dialer.DialerOpts{
			devtoolInfo.GetDevtoolPeerId(): {
				Address: linkUrl,
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

	// run the browser plugin host controller
	_, _, phRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&plugin_host_web.Config{
			HostConfig:   &plugin_host_controller.Config{},
			WebRuntimeId: initm.GetWebRuntimeId(),
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

	<-ctx.Done()
}

// formatBanner formats the full banner.
func formatBanner() string {
	return banner.FormatBanner()
}

// writeBanner writes the banner to the browser console.
func writeBanner() {
	defer func() {
		_ = recover()
	}()

	// write aperture banner
	js.Global().Get("console").Call(
		"log",
		"%c"+formatBanner(),
		"color:#ff3838;font-size:0.98em;font-family:monospace",
	)

	// clever note to anyone watching
	/*
		js.Global().Get("console").Call(
			"log",
			"%c"+"Oh. It's you... It's been a long time. How have you been?",
			// "color:#ff9a00;font-size:1.02em;font-family:monospace",
			"color:#27a7d8;font-size:0.8em;font-family:monospace",
		)
	*/
}

// getLocationOrigin returns the worker location URL.
func getLocationOrigin() string {
	global := js.Global()
	location := global.Get("location")
	origin := location.Get("origin")
	return origin.String()
}
