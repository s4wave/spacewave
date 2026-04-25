//go:build !js

package spacewave_launcher_controller

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/aperturerobotics/util/http"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/appversion"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// initUpdaterRoutine sets the updater routine on desktop platforms.
func (c *Controller) initUpdaterRoutine() {
	c.updaterRoutine.SetRoutine(c.checkAndDownloadUpdate)
}

// checkAndDownloadUpdate checks the current dist config for a version mismatch
// and downloads the new binary to the staging directory if needed.
func (c *Controller) checkAndDownloadUpdate(ctx context.Context) error {
	// detect if running inside a macOS .app bundle
	execPath, _ := os.Executable()
	isBundle, bundleRoot := detectAppBundle(execPath)

	// seed STAGED state from the sidecar manifest if a prior session left a
	// staged .app in place. Keeps the loading window and the recheck loop
	// aware of the staged version across restarts.
	c.seedStagedFromManifest()

	var info *spacewave_launcher.LauncherInfo
	for {
		var err error
		info, err = c.launcherInfoCtr.WaitValueChange(ctx, info, nil)
		if err != nil {
			return err
		}
		distConf := info.GetDistConfig()
		if distConf.GetRev() == 0 {
			continue
		}

		targetVersion := distConf.GetEntrypointVersion()
		if targetVersion == "" {
			continue
		}

		currentVersion := appversion.GetVersion()
		if targetVersion == currentVersion {
			// already up to date, clear any stale update state
			c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
				if li.GetUpdateState().GetPhase() == spacewave_launcher.UpdatePhase_UpdatePhase_IDLE {
					return false, nil
				}
				li.UpdateState = nil
				return true, nil
			})
			continue
		}

		platform := runtime.GOOS + "/" + runtime.GOARCH
		asset := distConf.GetEntrypointAssets()[platform]
		if asset == nil || asset.GetUrl() == "" {
			c.le.WithField("platform", platform).Debug("no entrypoint asset for platform")
			continue
		}

		// check if already staged and still fresh against the current
		// DistConfig. A staged version that no longer matches the advertised
		// entrypoint_version, or whose staged path has been removed, must be
		// wiped so the loop falls through to DOWNLOADING.
		if info.GetUpdateState().GetPhase() == spacewave_launcher.UpdatePhase_UpdatePhase_STAGED {
			stagedVersion := info.GetUpdateState().GetVersion()
			stagedPath := info.GetUpdateState().GetStagedPath()
			stagedFresh := stagedVersion == targetVersion && stagedPath != ""
			if stagedFresh {
				if _, err := os.Stat(stagedPath); err == nil {
					continue
				}
			}
			c.le.WithField("staged-version", stagedVersion).
				WithField("target-version", targetVersion).
				Info("staged update stale or missing, wiping and re-downloading")
			c.wipeStagedUpdate(stagedPath)
			c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
				li.UpdateState = nil
				return true, nil
			})
		}

		c.le.WithField("target-version", targetVersion).
			WithField("current-version", currentVersion).
			Info("entrypoint update available, downloading")

		if isBundle {
			c.le.WithField("bundle-root", bundleRoot).
				WithField("target-version", targetVersion).
				Info("macOS .app bundle detected, will sync via Manifest")
			if err := c.syncAppBundleManifest(ctx, asset, targetVersion, bundleRoot); err != nil {
				c.le.WithError(err).Warn("failed to sync .app Manifest")
				c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
					li.UpdateState = &spacewave_launcher.UpdateState{
						Phase:        spacewave_launcher.UpdatePhase_UpdatePhase_ERROR,
						Version:      targetVersion,
						ErrorMessage: err.Error(),
					}
					return true, nil
				})
				continue
			}
		}
		if isBundle {
			continue
		}
		if err := c.downloadAndStage(ctx, asset, targetVersion); err != nil {
			c.le.WithError(err).Warn("failed to download update")
			c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
				li.UpdateState = &spacewave_launcher.UpdateState{
					Phase:        spacewave_launcher.UpdatePhase_UpdatePhase_ERROR,
					Version:      targetVersion,
					ErrorMessage: err.Error(),
				}
				return true, nil
			})
			continue
		}
	}
}

// wipeStagedUpdate removes a stale staged raw binary or .app path and the
// sidecar manifest so the next loop iteration can re-enter DOWNLOADING against
// the current DistConfig. Errors are logged but not returned: the caller only
// needs the on-disk state to be consistent with "nothing staged" best-effort.
func (c *Controller) wipeStagedUpdate(stagedPath string) {
	if stagedPath != "" {
		if err := os.RemoveAll(stagedPath); err != nil && !os.IsNotExist(err) {
			c.le.WithError(err).WithField("staged-path", stagedPath).
				Warn("failed to remove stale staged update")
		}
	}
	stagingDir, err := getStagingDir()
	if err != nil {
		return
	}
	if err := removeStagedManifest(stagingDir); err != nil {
		c.le.WithError(err).Warn("failed to remove stale staged manifest")
	}
}

// seedStagedFromManifest reads the staging-dir sidecar manifest and, when a
// valid staged .app is still present on disk, publishes a STAGED UpdateState
// so later loop iterations can compare against the latest DistConfig without
// a redundant download. An unreadable manifest or missing staged path wipes
// the sidecar so we do not advertise stale state.
func (c *Controller) seedStagedFromManifest() {
	stagingDir, err := getStagingDir()
	if err != nil {
		return
	}
	manifest, err := readStagedManifest(stagingDir)
	if err != nil {
		c.le.WithError(err).Warn("clearing unreadable staged manifest")
		_ = removeStagedManifest(stagingDir)
		return
	}
	if manifest == nil {
		return
	}
	stagedPath := manifest.GetPath()
	if stagedPath == "" {
		_ = removeStagedManifest(stagingDir)
		return
	}
	if _, err := os.Stat(stagedPath); err != nil {
		_ = removeStagedManifest(stagingDir)
		return
	}
	c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
		li.UpdateState = &spacewave_launcher.UpdateState{
			Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
			Version:    manifest.GetVersion(),
			StagedPath: stagedPath,
		}
		return true, nil
	})
}

// downloadAndStage downloads the raw-binary archive from the asset URL,
// verifies the archive hash, extracts the platform binary, and stages it.
func (c *Controller) downloadAndStage(
	ctx context.Context,
	asset *spacewave_launcher.EntrypointAsset,
	version string,
) error {
	stagingDir, err := getStagingDir()
	if err != nil {
		return errors.Wrap(err, "get staging dir")
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return errors.Wrap(err, "create staging dir")
	}

	// set downloading state
	c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
		li.UpdateState = &spacewave_launcher.UpdateState{
			Phase:   spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING,
			Version: version,
		}
		return true, nil
	})

	stagedPath := filepath.Join(stagingDir, rawEntrypointBinaryName())
	downloadPath := filepath.Join(stagingDir, "entrypoint-"+version+rawEntrypointArchiveSuffix()+".downloading")

	// clean up partial download on failure
	defer func() {
		_ = os.Remove(downloadPath)
	}()

	req, err := http.NewRequestWithContext(ctx, "GET", asset.GetUrl(), nil)
	if err != nil {
		return errors.Wrap(err, "create request")
	}
	resp, err := http.DoRequest(c.le.WithField("routine", "updater"), http.DefaultClient, req, true)
	if err != nil {
		return errors.Wrap(err, "download binary")
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(downloadPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return errors.Wrap(err, "create download file")
	}

	hasher := sha256.New()
	writer := io.MultiWriter(f, hasher)

	expectedSize := asset.GetSize()
	var written int64
	buf := make([]byte, 32*1024)
	for {
		nr, readErr := resp.Body.Read(buf)
		if nr > 0 {
			nw, writeErr := writer.Write(buf[:nr])
			if writeErr != nil {
				_ = f.Close()
				return errors.Wrap(writeErr, "write to staging file")
			}
			written += int64(nw)

			// update progress
			if expectedSize > 0 {
				progress := min(uint32(written*100/int64(expectedSize)), 100)
				c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
					us := li.GetUpdateState()
					if us == nil || us.GetPhase() != spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING {
						return false, nil
					}
					if us.GetDownloadProgress() == progress {
						return false, nil
					}
					li.UpdateState = &spacewave_launcher.UpdateState{
						Phase:            spacewave_launcher.UpdatePhase_UpdatePhase_DOWNLOADING,
						Version:          version,
						DownloadProgress: progress,
					}
					return true, nil
				})
			}
		}
		if readErr != nil {
			if readErr == io.EOF {
				break
			}
			_ = f.Close()
			return errors.Wrap(readErr, "read response body")
		}
	}

	if err := f.Close(); err != nil {
		return errors.Wrap(err, "close staging file")
	}

	// verify sha256
	expectedHash := asset.GetSha256()
	if len(expectedHash) != 0 {
		actualHash := hasher.Sum(nil)
		if !bytes.Equal(actualHash, expectedHash) {
			return errors.New("sha256 hash mismatch")
		}
	}

	if err := os.Remove(stagedPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove stale staged binary")
	}
	if err := extractRawEntrypointArchive(downloadPath, stagedPath); err != nil {
		return err
	}
	if err := os.Remove(downloadPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove downloaded archive")
	}

	if err := writeStagedManifest(stagingDir, &spacewave_launcher.StagedManifest{
		Version:       version,
		Path:          stagedPath,
		SignatureHash: expectedHash,
	}); err != nil {
		return errors.Wrap(err, "write staged manifest")
	}

	// set staged state
	c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
		li.UpdateState = &spacewave_launcher.UpdateState{
			Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
			Version:    version,
			StagedPath: stagedPath,
		}
		return true, nil
	})

	c.le.WithField("version", version).
		WithField("staged-path", stagedPath).
		Info("entrypoint update staged and ready")

	return nil
}

func rawEntrypointBinaryName() string {
	if runtime.GOOS == "windows" {
		return "spacewave.exe"
	}
	return "spacewave"
}

func rawEntrypointArchiveSuffix() string {
	if runtime.GOOS == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func extractRawEntrypointArchive(archivePath, stagedPath string) error {
	if runtime.GOOS == "windows" {
		return extractRawEntrypointZip(archivePath, stagedPath)
	}
	return extractRawEntrypointTarGz(archivePath, stagedPath)
}

func extractRawEntrypointTarGz(archivePath, stagedPath string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return errors.Wrap(err, "open downloaded archive")
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return errors.Wrap(err, "open gzip archive")
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	return extractRawEntrypointFromTar(tr, stagedPath)
}

func extractRawEntrypointFromTar(tr *tar.Reader, stagedPath string) error {
	name := rawEntrypointBinaryName()
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "read tar archive")
		}
		if hdr.FileInfo().IsDir() || filepath.Base(hdr.Name) != name {
			continue
		}
		return writeExtractedRawEntrypoint(tr, stagedPath)
	}
	return errors.Errorf("raw entrypoint %q not found in archive", name)
}

func extractRawEntrypointZip(archivePath, stagedPath string) error {
	zr, err := zip.OpenReader(archivePath)
	if err != nil {
		return errors.Wrap(err, "open zip archive")
	}
	defer zr.Close()

	name := rawEntrypointBinaryName()
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() || filepath.Base(zf.Name) != name {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return errors.Wrap(err, "open zipped entrypoint")
		}
		defer rc.Close()
		return writeExtractedRawEntrypoint(rc, stagedPath)
	}
	return errors.Errorf("raw entrypoint %q not found in archive", name)
}

func writeExtractedRawEntrypoint(src io.Reader, stagedPath string) error {
	tmpPath := stagedPath + ".extracting"
	defer os.Remove(tmpPath)

	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return errors.Wrap(err, "create staged entrypoint")
	}
	if _, err := io.Copy(dst, src); err != nil {
		_ = dst.Close()
		return errors.Wrap(err, "extract raw entrypoint")
	}
	if err := dst.Close(); err != nil {
		return errors.Wrap(err, "close staged entrypoint")
	}
	if err := os.Rename(tmpPath, stagedPath); err != nil {
		return errors.Wrap(err, "rename staged entrypoint")
	}
	return nil
}
