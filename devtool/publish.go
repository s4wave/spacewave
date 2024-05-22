//go:build !js

package devtool

import (
	"context"
	"strings"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
)

// PublishProject publishes a bundle to a repository.
func (a *DevtoolArgs) PublishProject(ctx context.Context) error {
	// init repo root and storage directories
	le := a.Logger

	a.Watch = false                                       // explicitly disable watching during dist
	a.BuildType = string(bldr_manifest.BuildType_RELEASE) // explicitly set release build type
	a.MinifyEntrypoint = true                             // explicitly minify entrypoint during dist

	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	_ = repoRoot
	le.Infof("starting with state dir: %s", stateDir)

	// initialize the storage + bus
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer b.Release()

	// write the banner
	writeBanner()

	// execute the project controller
	projWatcher, projWatcherRef, err := b.StartProjectController(
		ctx,
		b.GetBus(),
		repoRoot,
		a.ConfigPath,
		"",
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

	// publish
	return projCtrl.PublishTargets(
		ctx,
		a.Remote,
		strings.Split(a.PublishCsv, ","),
		bldr_manifest.BuildType(a.BuildType),
	)
}
