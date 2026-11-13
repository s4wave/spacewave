//go:build !js

package devtool

import (
	"context"
	"os"
	"strings"

	devtool_store "github.com/s4wave/spacewave/bldr/devtool/store"
	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	"github.com/s4wave/spacewave/db/world"
	"github.com/sirupsen/logrus"
)

// BuildProject builds one of the targets defined in the project configuration.
func (a *DevtoolArgs) BuildProject(ctx context.Context) (err error) {
	// Resolve the project and state directories for a single build.
	le := a.Logger
	a.Watch = a.BuildWatch
	repoRoot, stateDir, err := a.InitRepoRoot()
	if err != nil {
		return err
	}
	le.Infof("starting with state dir: %s", stateDir)

	// Retain the build bus and report the command's final result on every exit.
	b, err := a.buildDevtoolBus(ctx, le, repoRoot, stateDir, a.Watch)
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

	// Hold the builders while watching or exporting.
	var hold func(context.Context, []string) error
	if a.Watch || a.ExportDir != "" {
		ws := b.GetWorldState()
		hold = func(ctx context.Context, manifestIDs []string) error {
			return a.holdBuild(ctx, le, ws, manifestIDs)
		}
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
		a.ManifestRevision,
		hold,
	)
}

// holdBuild exports the latest manifest sets of manifestIDs to the export
// directory, then, when the build watches, again after each World change that
// moves one of their roots, until ctx ends. A watched export that fails is
// logged and retried on the next change, leaving the previous export in place.
func (a *DevtoolArgs) holdBuild(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	manifestIDs []string,
) error {
	var exported string
	for {
		// Read the World's position before checking it, so no change is missed.
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			return err
		}

		// Export the sets when their roots moved.
		if a.ExportDir != "" {
			exported, err = a.exportChanged(ctx, le, ws, manifestIDs, exported)
			if err != nil && !a.Watch {
				return err
			}
			if err != nil {
				le.WithError(err).Warn("export failed; previous export kept")
			}
		}
		if !a.Watch {
			return nil
		}

		// Wait for the next World change.
		if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

// exportChanged exports the latest sets of manifestIDs when their roots differ
// from prev, and returns the roots now exported.
func (a *DevtoolArgs) exportChanged(
	ctx context.Context,
	le *logrus.Entry,
	ws world.WorldState,
	manifestIDs []string,
	prev string,
) (string, error) {
	// Collect the latest set of each manifest and the roots they name.
	var refs []*bldr_manifest.ManifestRef
	var roots strings.Builder
	for _, id := range manifestIDs {
		set, err := devtool_store.CollectLatestManifestSet(ctx, ws, id)
		if err != nil {
			return prev, err
		}
		for _, ref := range set {
			roots.WriteString(ref.GetManifestRef().GetRootRef().MarshalString())
			roots.WriteByte(' ')
		}
		refs = append(refs, set...)
	}
	if roots.String() == prev {
		return prev, nil
	}

	// Write the export and name each manifest it holds.
	if err := devtool_store.ExportManifestSets(ctx, le, ws, refs, a.ExportDir); err != nil {
		return prev, err
	}
	for _, ref := range refs {
		le.WithField("manifest-id", ref.GetMeta().GetManifestId()).
			WithField("platform-id", ref.GetMeta().GetPlatformId()).
			WithField("manifest-rev", ref.GetMeta().GetRev()).
			WithField("ref", ref.GetManifestRef().GetRootRef().MarshalString()).
			Info("exported manifest set")
	}
	return roots.String(), nil
}

// buildCommandSummary describes the requested targets and explicit overrides.
func buildCommandSummary(buildCSV, buildType, remote, targetsCSV string) string {
	// Collect the target list, build type, remote, and explicit target override
	// that are set.
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

	// Join the summary fields with spaces.
	return strings.Join(parts, " ")
}
