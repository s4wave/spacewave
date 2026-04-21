//go:build !js

package devtool

import (
	"context"
	"strings"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// BuildProject builds one of the targets defined in the project configuration.
func (a *DevtoolArgs) BuildProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger

	a.Watch = false // explicitly disable watching during build

	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, repoRoot, stateDir, a.Watch)
	if err != nil {
		return err
	}

	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath); err != nil {
		return err
	}
	defer b.Release()

	// write the banner
	writeBanner()

	// execute the project controller
	// compiles the plugins and stores them in the devtool bus world
	projWatcher, projWatcherRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		repoRoot,
		a.ConfigPath,
		"",
		nil,
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

	// build the targets
	var targetsOverride []string
	if a.TargetsCsv != "" {
		targetsOverride = strings.Split(a.TargetsCsv, ",")
	}
	return projCtrl.BuildTargets(
		ctx,
		a.Remote,
		strings.Split(a.BuildCsv, ","),
		bldr_manifest.BuildType(a.BuildType),
		targetsOverride,
	)
}
