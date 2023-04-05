package devtool

import (
	"context"
	"errors"

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
	b, err := BuildDevtoolBus(ctx, le, stateDir, a.Watch, false)
	if err != nil {
		return err
	}

	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum); err != nil {
		return err
	}
	defer b.Release()

	// write the banner
	writeBanner()

	// TODO
	return errors.New("TODO")
}
