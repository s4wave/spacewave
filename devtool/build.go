package devtool

import (
	"context"
	"os"
	"path"
	"strings"

	plugin "github.com/aperturerobotics/bldr/plugin"
)

// BuildProject builds a dist bundle of the project to dist/ with the given platform ID.
func (a *DevtoolArgs) BuildProject(ctx context.Context) error {
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

	// read the bldr go.mod
	baseGoMod, err := os.ReadFile(path.Join(b.GetDistSrcDir(), "go.mod"))
	if err != nil {
		return err
	}

	// read the bldr go.sum
	baseGoSum, err := os.ReadFile(path.Join(b.GetDistSrcDir(), "go.sum"))
	if err != nil {
		return err
	}

	// write the banner
	writeBanner()

	// execute the project controller
	// compiles the plugins and stores them in the devtool bus world
	projWatcher, projWatcherRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		false,
		repoRoot,
		a.ConfigPath,
		"", // TODO empty platform id
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

	_ = appID
	_ = baseGoMod
	_ = baseGoSum

	// TODO lookup the build config

	// cleanup: remove working path
	/*
		if !a.DisableCleanup {
			_ = os.RemoveAll(buildRoot)
		}
	*/
	return projCtrl.BuildTargets(ctx, strings.Split(",", a.BuildCsv))
}
