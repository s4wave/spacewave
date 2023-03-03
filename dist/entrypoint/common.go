package dist_entrypoint

import (
	"context"

	plugin "github.com/aperturerobotics/bldr/plugin"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
	"github.com/sirupsen/logrus"
	"golang.org/x/exp/slices"
)

// Execute builds the bus & starts common controllers.
func Execute(
	ctx context.Context,
	le *logrus.Entry,
	appID,
	distPlatformID string,
	staticPluginManifests []*plugin.StaticPlugin,
	startPlugins []string,
) error {
	storageRoot, err := DetermineStorageRoot(appID)
	if err != nil {
		le.WithError(err).Warn("unable to determine storage root, using current dir")
		storageRoot = "./state"
	}
	distBus, err := BuildDistBus(ctx, le, appID, distPlatformID, storageRoot)
	if err != nil {
		le.WithError(err).Fatal("unable to initialize application")
	}
	defer distBus.Release()

	le.Info("host is ready")
	writeBanner()

	// Load any embedded plugin manifests into the world.
	// Do not overwrite any existing plugin manifests.
	errCh := make(chan error, len(staticPluginManifests))
	for _, staticPlugin := range staticPluginManifests {
		pluginID := staticPlugin.Manifest.GetMeta().GetPluginId()
		startPlugin := slices.Contains(startPlugins, pluginID)
		relStaticPlugin, err := distBus.ExecStaticPlugin(
			ctx,
			le,
			controller.NewInfo(
				"exec-static-plugin/"+pluginID,
				semver.MustParse("0.0.1"),
				"exec plugin: "+pluginID,
			),
			staticPlugin,
			startPlugin,
			func(err error) {
				errCh <- err
			},
		)
		if err != nil {
			le.WithError(err).Fatal("unable to load embedded plugin")
		}
		defer relStaticPlugin()
	}

	select {
	case <-ctx.Done():
		return context.Canceled
	case err := <-errCh:
		le.WithError(err).Fatal("error loading embedded plugin")
		return err
	}
}
