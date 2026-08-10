//go:build !js

package devtool

import (
	"context"
	"os"
	"path/filepath"

	bldr_manifest "github.com/s4wave/spacewave/bldr/manifest"
	bldr_manifest_world "github.com/s4wave/spacewave/bldr/manifest/world"
	bldr_platform "github.com/s4wave/spacewave/bldr/platform"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
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
	b, err := BuildDevtoolBus(ctx, le, repoRoot, stateDir, a.Watch)
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
		runCtx, cancelRun := context.WithCancel(ctx)
		proc := NewCLIProcessSupervisor(runCtx, le, binaryPath, args)
		if err := proc.Start(); err != nil {
			cancelRun()
			return err
		}

		// watch for manifest changes in the world
		rebuildCh := make(chan error, 1)
		if a.Watch {
			go func() {
				watchErr := a.watchManifestChanges(runCtx, b, manifestID, platformID, &lastRev)
				if watchErr != nil {
					if runCtx.Err() == nil {
						le.WithError(watchErr).Warn("manifest watch error")
						select {
						case rebuildCh <- watchErr:
						default:
						}
					}
					return
				}
				select {
				case rebuildCh <- nil:
				default:
				}
			}()
		}

		select {
		case <-proc.Done():
			err := proc.Wait()
			// subprocess exited on its own (not killed by us)
			// propagate exit code to parent
			cancelRun()
			return exitWithChildCode(err)

		case watchErr := <-rebuildCh:
			if watchErr != nil {
				cancelRun()
				_ = proc.Terminate()
				return watchErr
			}
			// manifest rebuilt, kill subprocess and restart
			le.Info("manifest rebuilt, restarting CLI...")
			cancelRun()
			_ = proc.Terminate()

		case <-ctx.Done():
			cancelRun()
			return exitWithChildCode(proc.Terminate())
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
