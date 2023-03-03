//go:build !js
// +build !js

package dist_entrypoint

import (
	"context"
	"os"
	"os/signal"

	dist_platform "github.com/aperturerobotics/bldr/dist/platform"
	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/sirupsen/logrus"
)

// DistPlatformID is the distribution platform ID.
const DistPlatformID = dist_platform.DistPlatformID_NATIVE

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

	if err := Execute(ctx, le, appID, DistPlatformID, staticPluginManifests, startPlugins); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
