//go:build !js

package devtool

import (
	"context"
	"strings"

	"github.com/pkg/errors"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// PublishProject publishes a bundle to a repository.
func (a *DevtoolArgs) PublishProject(ctx context.Context) error {
	targets, err := parsePublishTargets(a.PublishCsv)
	if err != nil {
		return err
	}

	a.Watch = false
	a.BuildType = string(bldr_manifest.BuildType_RELEASE)
	a.MinifyEntrypoint = true

	le := a.Logger

	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	b, err := BuildDevtoolBus(ctx, le, repoRoot, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer b.Release()

	writeBanner()

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

	projCtrl, err := projWatcher.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	return projCtrl.PublishTargets(
		ctx,
		a.Remote,
		targets,
		bldr_manifest.BuildType(a.BuildType),
	)
}

// parsePublishTargets returns a non-empty normalized target selection.
func parsePublishTargets(value string) ([]string, error) {
	parts := strings.Split(value, ",")
	targets := make([]string, 0, len(parts))
	for _, part := range parts {
		target := strings.TrimSpace(part)
		if target == "" {
			return nil, errors.New("publish target must not be empty")
		}
		targets = append(targets, target)
	}
	return targets, nil
}
