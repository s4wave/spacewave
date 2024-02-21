//go:build js
// +build js

package main

import (
	"context"
	"syscall/js"

	"github.com/aperturerobotics/bldr/banner"
	"github.com/aperturerobotics/bldr/core"
	plugin_host_controller "github.com/aperturerobotics/bldr/plugin/host/controller"
	plugin_host_web "github.com/aperturerobotics/bldr/plugin/host/web"
	"github.com/aperturerobotics/bldr/storage"
	browser_storage "github.com/aperturerobotics/bldr/storage/browser"
	web_entrypoint_browser "github.com/aperturerobotics/bldr/web/entrypoint/browser"
	"github.com/aperturerobotics/bldr/web/plugin/browser"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// LogLevel is the default log level to use.
var LogLevel = logrus.DebugLevel

// TODO: set app id with ldflags
var appID = "aperture"

func main() {
	log := logrus.New()
	log.SetLevel(LogLevel)
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	le := logrus.NewEntry(log)

	initm, err := web_entrypoint_browser.ReadInitMessage()
	if err != nil {
		le.WithError(err).Fatal("failed to read init message")
	}
	writeBanner()

	ctx := context.Background()
	b, sr, err := core.NewCoreBus(ctx, le)
	if err != nil {
		le.Fatal(err.Error())
	}
	sr.AddFactory(browser.NewFactory(b))
	sr.AddFactory(plugin_host_web.NewFactory(b))

	// run the browser storage
	browserStorage := browser_storage.BuildStorage(b, "")
	storageRel := storage.ExecuteStorage(ctx, b, le, browserStorage, appID)
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
