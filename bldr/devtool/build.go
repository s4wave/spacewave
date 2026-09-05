//go:build !js

package devtool

import (
	"context"
	"os"
	"strings"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// BuildProject builds one of the targets defined in the project configuration.
func (a *DevtoolArgs) BuildProject(ctx context.Context) (err error) {
	// Resolve the project and state directories for a single build.
	le := a.Logger
	a.Watch = false
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// Retain the build bus and report the command's final result on every exit.
	b, err := BuildDevtoolBus(ctx, le, repoRoot, stateDir, a.Watch)
	if err != nil {
		return err
	}
	defer b.Release()
	commandLogFile := a.commandLogFile()
	b.setCommandStartingWithLogFile("build", "initializing build", commandLogFile)
	ctx, stopTUI := a.startDevtoolTUI(ctx, b.GetStatusProducer(), "")
	defer func() {
		b.finishCommandWithLogFile(ctx, "build", commandLogFile, err)
		stopTUI()
	}()

	// Synchronize the compiler sources before evaluating project targets.
	err = b.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath)
	if err != nil {
		return err
	}

	// Display compiler identity before project build diagnostics.
	a.writeBannerTo(os.Stderr)

	// Resolve dependency manifests through the same remote as the requested
	// targets. Builds need on-demand compilation without application startup.
	projWatcher, projWatcherRef, err := b.StartProjectControllerWithStartup(
		ctx,
		b.GetBus(),
		repoRoot,
		a.ConfigPath,
		a.Remote,
		nil,
		false,
	)
	if err != nil {
		return err
	}
	defer projWatcherRef.Release()

	// Wait for evaluated project configuration before selecting targets.
	projCtrl, err := projWatcher.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// Apply explicit target and compiler-policy overrides to this build.
	var targetsOverride []string
	if a.TargetsCsv != "" {
		targetsOverride = strings.Split(a.TargetsCsv, ",")
	}
	buildPolicyOverride, err := a.BuildPolicyOverride()
	if err != nil {
		return err
	}
	// Publish build progress while the project controller compiles its targets.
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
		buildPolicyOverride,
	)
}

// buildCommandSummary describes the requested targets and explicit overrides.
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
