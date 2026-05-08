//go:build !js

package devtool

import (
	"context"
	"strings"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// BuildProject builds one of the targets defined in the project configuration.
func (a *DevtoolArgs) BuildProject(ctx context.Context) (err error) {
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
	defer b.Release()
	commandLogFile := a.commandLogFile()
	b.setCommandStartingWithLogFile("build", "initializing build", commandLogFile)
	stopTUI, err := a.startTUIRunner(ctx, b.GetStatusProducer())
	if err != nil {
		return err
	}
	defer func() {
		b.finishCommandThenStopTUI(ctx, "build", commandLogFile, err, stopTUI)
	}()

	err = b.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath)
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
	b.setCommandRunningWithLogFile(
		"build",
		buildCommandSummary(a.BuildCsv, a.BuildType, a.Remote, a.TargetsCsv),
		commandLogFile,
	)
	return projCtrl.BuildTargets(
		ctx,
		a.Remote,
		strings.Split(a.BuildCsv, ","),
		bldr_manifest.BuildType(a.BuildType),
		targetsOverride,
	)
}

func buildCommandSummary(buildCSV, buildType, remote, targetsCSV string) string {
	parts := []string{"building targets"}
	if buildCSV != "" {
		parts = append(parts, strings.TrimSpace(buildCSV))
	}
	if buildType != "" {
		parts = append(parts, "build-type="+strings.TrimSpace(buildType))
	}
	if remote != "" {
		parts = append(parts, "remote="+strings.TrimSpace(remote))
	}
	if targetsCSV != "" {
		parts = append(parts, "targets="+strings.TrimSpace(targetsCSV))
	}
	return strings.Join(parts, " ")
}
