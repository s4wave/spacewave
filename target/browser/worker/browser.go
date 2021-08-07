// +build js

package main

import (
	"context"

	"github.com/aperturerobotics/bldr/runtime"
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

	// TODO: determine if this is the root WebView...
	// TODO: wait for init: message if gopherjs

	ctx := context.Background()
	rt, err := NewRuntime(ctx, le, NewWebView(ctx, "id", true))
	if err != nil {
		le.Fatal(err.Error())
	}
	if err := runtime.Run(ctx, le, rt); err != nil {
		le.Fatal(err.Error())
	}
}
