//go:build !js
// +build !js

package dist_entrypoint

import (
	"context"
	"os"
	"os/signal"

	"github.com/aperturerobotics/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// Main runs the default main entrypoint for a native program.
func Main(
	appID string,
	staticPluginManifests []*plugin.StaticPlugin,
	startPlugins []string,
) {
	log := logrus.New()
	log.SetFormatter(&logrus.TextFormatter{
		DisableColors:    false,
		DisableTimestamp: false,
	})
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	ctx, ctxCancel := signal.NotifyContext(context.Background(), os.Interrupt, os.Kill)
	defer ctxCancel()

	if err := Execute(ctx, le, appID, staticPluginManifests, startPlugins); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
