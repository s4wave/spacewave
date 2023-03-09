package devtool

import (
	"context"
	"errors"
	"os"
	"path"

	dist_platform "github.com/aperturerobotics/bldr/dist/platform"
	plugin "github.com/aperturerobotics/bldr/plugin"
)

// PublishProject publishes a bundle to a repository.
func (a *DevtoolArgs) PublishProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger

	a.Watch = false                                // explicitly disable watching during dist
	a.BuildType = string(plugin.BuildType_RELEASE) // explicitly set release build type
	a.MinifyEntrypoint = true                      // explicitly minify entrypoint during dist

	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch)
	if err != nil {
		return err
	}

	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum); err != nil {
		return err
	}
	defer b.Release()

	// write the banner
	writeBanner()

	// determine the plugin platform ID corresponding to the given dist platform ID
	pluginPlatformID, err := dist_platform.GetPluginPlatformID(a.DistPlatformID)
	if err != nil {
		return err
	}

	// build the working dir
	distRoot := path.Join(stateDir, "dist")
	buildRoot := path.Join(distRoot, "build", a.DistPlatformID)
	outputRoot := path.Join(a.OutputPath, "dist", a.DistPlatformID)

	// create / clean the directories
	if err := os.MkdirAll(buildRoot, 0755); err != nil {
		return err
	}
	if _, err := os.Stat(outputRoot); err == nil {
		if err := os.RemoveAll(outputRoot); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(outputRoot, 0755); err != nil {
		return err
	}

	// execute the project controller
	// compiles the plugins and stores them in the devtool bus world
	projWatcher, projWatcherRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		false,
		repoRoot,
		a.ConfigPath,
		pluginPlatformID,
		a.BuildType,
	)
	if err != nil {
		return err
	}
	defer projWatcherRef.Release()

	// get the project controller from the watcher
	projCtrl, err := projWatcher.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// get the project config
	projCtrlConf := projCtrl.GetConfig()
	projConf := projCtrlConf.GetProjectConfig()
	appID := projConf.GetId()

	// TODO: build plugins
	// TODO: build dist bundles
	// targetIDs :=
	_ = appID

	// cleanup: remove working path
	if !a.DisableCleanup {
		_ = os.RemoveAll(buildRoot)
	}
	return errors.New("TODO")
}
