//go:build js
// +build js

package main

import (
	"context"

	"github.com/aperturerobotics/bldr/core"
	"github.com/aperturerobotics/bldr/storage"
	browser_storage "github.com/aperturerobotics/bldr/storage/browser"
	"github.com/aperturerobotics/bldr/web/plugin/browser"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// LogLevel is the default log level to use.
var LogLevel = logrus.DebugLevel

// TODO: set app id
var appID = "aperture"

func main() {
	log := logrus.New()
	log.SetLevel(LogLevel)
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    true,
		DisableTimestamp: true,
	})
	le := logrus.NewEntry(log)

	initm, err := readInitMessage()
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
	<-ctx.Done()
	rtRef.Release()
}
