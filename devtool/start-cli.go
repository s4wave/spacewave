//go:build !js

package devtool

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	bldr_manifest "github.com/aperturerobotics/bldr/manifest"
	bldr_manifest_world "github.com/aperturerobotics/bldr/manifest/world"
	bldr_platform "github.com/aperturerobotics/bldr/platform"
	unixfs_sync "github.com/aperturerobotics/hydra/unixfs/sync"
	"github.com/sirupsen/logrus"
)

// ExecuteCliProject builds and runs a CLI manifest as a subprocess.
//
// Watches for manifest changes and restarts the subprocess automatically.
// Forwards signals to the child and propagates its exit code.
func (a *DevtoolArgs) ExecuteCliProject(ctx context.Context, manifestID string, args []string) error {
	le := a.Logger
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
	defer b.Release()

	// sync dist sources
	if err := b.SyncDistSources(a.BldrVersion, a.BldrVersionSum, a.BldrSrcPath); err != nil {
		return err
	}

	// write the banner
	writeBanner()

	// start the project controller
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

	// get the project controller
	projCtrl, err := projWatcher.GetProjectController().WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	// build the CLI manifest
	le.Infof("building CLI manifest: %s", manifestID)
	manifestRefs, _, err := projCtrl.BuildManifests(
		ctx,
		a.Remote,
		[]string{manifestID},
		bldr_manifest.BuildType(a.BuildType),
		nil,
	)
	if err != nil {
		return err
	}
	if len(manifestRefs) == 0 {
		return nil
	}
	manifestRef := manifestRefs[0]

	// determine checkout path
	cliDir := filepath.Join(stateDir, "cli", manifestID)
	distPath := filepath.Join(cliDir, "dist")
	if err := os.MkdirAll(distPath, 0o755); err != nil {
		return err
	}

	// checkout the manifest to disk
	le.Infof("checking out CLI binary to: %s", distPath)
	manifest, err := bldr_manifest_world.CheckoutManifest(
		ctx,
		le,
		b.GetWorldState().AccessWorldState,
		manifestRef.GetManifestRef(),
		distPath,
		"",
		unixfs_sync.DeleteMode_DeleteMode_BEFORE,
		nil,
		nil,
	)
	if err != nil {
		return err
	}

	// resolve entrypoint binary path
	entrypoint := manifest.GetEntrypoint()
	binaryPath := filepath.Join(distPath, entrypoint)

	// ensure executable
	if err := os.Chmod(binaryPath, 0o755); err != nil {
		return err
	}

	le.Infof("starting CLI: %s %v", entrypoint, args)

	// run the subprocess, restart on manifest changes
	return a.runCliSubprocess(ctx, le, b, manifestID, binaryPath, args)
}

// runCliSubprocess runs the CLI binary as a subprocess and watches for rebuilds.
func (a *DevtoolArgs) runCliSubprocess(
	ctx context.Context,
	le *logrus.Entry,
	b *DevtoolBus,
	manifestID, binaryPath string,
	args []string,
) error {
	np, err := bldr_platform.ParseNativePlatform("desktop")
	if err != nil {
		return err
	}
	platformID := np.GetPlatformID()

	// track the last known manifest revision
	var lastRev uint64

	for {
		// start subprocess
		proc := exec.CommandContext(ctx, binaryPath, args...)
		proc.Stdin = os.Stdin
		proc.Stdout = os.Stdout
		proc.Stderr = le.WriterLevel(logrus.DebugLevel)

		if err := proc.Start(); err != nil {
			return err
		}

		// wait for subprocess to exit OR a manifest rebuild
		procDone := make(chan error, 1)
		go func() {
			procDone <- proc.Wait()
		}()

		// watch for manifest changes in the world
		rebuildCh := make(chan struct{}, 1)
		if a.Watch {
			go func() {
				watchErr := a.watchManifestChanges(ctx, b, manifestID, platformID, &lastRev)
				if watchErr != nil && ctx.Err() == nil {
					le.WithError(watchErr).Warn("manifest watch error")
				}
				select {
				case rebuildCh <- struct{}{}:
				default:
				}
			}()
		}

		select {
		case err := <-procDone:
			// subprocess exited on its own (not killed by us)
			// propagate exit code to parent
			return exitError(err)

		case <-rebuildCh:
			// manifest rebuilt, kill subprocess and restart
			le.Info("manifest rebuilt, restarting CLI...")
			killProcess(proc)
			<-procDone

		case <-ctx.Done():
			killProcess(proc)
			err := <-procDone
			return exitError(err)
		}

		// collect the updated manifest ref from the world
		distPath := filepath.Dir(binaryPath)
		manifests, _, err := bldr_manifest_world.CollectManifestsForManifestID(
			ctx,
			b.GetWorldState(),
			manifestID,
			[]string{platformID},
			b.GetPluginHostObjectKey(),
		)
		if err != nil {
			return err
		}
		if len(manifests) == 0 {
			le.Warn("no manifests found after rebuild")
			continue
		}

		// re-checkout the updated manifest
		le.Info("checking out updated CLI binary...")
		manifest, err := bldr_manifest_world.CheckoutManifest(
			ctx,
			le,
			b.GetWorldState().AccessWorldState,
			manifests[0].ManifestRef,
			distPath,
			"",
			unixfs_sync.DeleteMode_DeleteMode_BEFORE,
			nil,
			nil,
		)
		if err != nil {
			return err
		}

		// update binary path in case entrypoint changed
		binaryPath = filepath.Join(distPath, manifest.GetEntrypoint())
		if err := os.Chmod(binaryPath, 0o755); err != nil {
			return err
		}
	}
}

// watchManifestChanges watches the world state for changes to a manifest.
// Blocks until a new revision is detected or the context is canceled.
func (a *DevtoolArgs) watchManifestChanges(
	ctx context.Context,
	b *DevtoolBus,
	manifestID, platformID string,
	lastRev *uint64,
) error {
	ws := b.GetWorldState()
	objKey := b.GetPluginHostObjectKey()

	for {
		seqno, err := ws.GetSeqno(ctx)
		if err != nil {
			return err
		}

		manifests, _, err := bldr_manifest_world.CollectManifestsForManifestID(
			ctx,
			ws,
			manifestID,
			[]string{platformID},
			objKey,
		)
		if err != nil {
			return err
		}

		if len(manifests) > 0 {
			rev := manifests[0].GetRev()
			if *lastRev == 0 {
				*lastRev = rev
			} else if rev > *lastRev {
				*lastRev = rev
				return nil // new version detected
			}
		}

		// wait for world state to change
		if _, err := ws.WaitSeqno(ctx, seqno+1); err != nil {
			return err
		}
	}
}

// killProcess sends SIGTERM to the process and waits briefly, then SIGKILL.
func killProcess(proc *exec.Cmd) {
	if proc.Process == nil {
		return
	}
	_ = proc.Process.Signal(syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		proc.Wait()
		close(done)
	}()
	timeout := time.NewTimer(3 * time.Second)
	select {
	case <-done:
		timeout.Stop()
	case <-timeout.C:
		_ = proc.Process.Kill()
	}
}

// exitError extracts the exit code from an exec error.
// Returns nil for success, the original error otherwise.
func exitError(err error) error {
	if err == nil {
		return nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	return err
}
