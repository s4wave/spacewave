//go:build js
// +build js

package main

import (
	"context"

	"github.com/aperturerobotics/bldr/core"
	"github.com/aperturerobotics/bldr/target/browser"
	"github.com/aperturerobotics/controllerbus/controller/loader"
	"github.com/aperturerobotics/controllerbus/controller/resolver"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// LogLevel is the default log level to use.
var LogLevel = logrus.DebugLevel

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

	// run the browser runtime controller
	_, _, rtRef, err := loader.WaitExecControllerRunning(
		ctx,
		b,
		resolver.NewLoadControllerWithConfig(&browser.Config{
			RuntimeId: initm.GetRuntimeId(),
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
