//go:build !js

package spacewave_launcher_controller

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/aperturerobotics/util/http"
	"github.com/pkg/errors"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
)

// syncAppBundleManifest downloads a macOS .app bundle archive from the asset
// URL, extracts it to the staging directory preserving xattrs, and sets the
// STAGED update state. The archive is a tar.gz containing the .app directory
// tree with xattr headers for code signature preservation.
func (c *Controller) syncAppBundleManifest(
	ctx context.Context,
	asset *spacewave_launcher.EntrypointAsset,
	version string,
	bundleRoot string,
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

	downloadPath := filepath.Join(stagingDir, "entrypoint-"+version+".tar.gz")
	defer func() {
		_ = os.Remove(downloadPath)
	}()

	// download the archive
	req, err := http.NewRequestWithContext(ctx, "GET", asset.GetUrl(), nil)
	if err != nil {
		return errors.Wrap(err, "create request")
	}
	resp, err := http.DoRequest(c.le.WithField("routine", "updater-appbundle"), http.DefaultClient, req, true)
	if err != nil {
		return errors.Wrap(err, "download .app archive")
	}
	defer resp.Body.Close()

	f, err := os.OpenFile(downloadPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
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
				return errors.Wrap(writeErr, "write to download file")
			}
			written += int64(nw)

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
		return errors.Wrap(err, "close download file")
	}

	// verify sha256
	expectedHash := asset.GetSha256()
	if len(expectedHash) != 0 {
		actualHash := hasher.Sum(nil)
		if !bytes.Equal(actualHash, expectedHash) {
			return errors.New("sha256 hash mismatch on .app archive")
		}
	}

	// extract the archive to staging dir, preserving xattrs
	stagedAppDir := filepath.Join(stagingDir, "Spacewave-"+version+".app")
	if err := os.RemoveAll(stagedAppDir); err != nil {
		return errors.Wrap(err, "clean staged .app dir")
	}

	if err := extractTarGzWithXattrs(downloadPath, stagingDir); err != nil {
		return errors.Wrap(err, "extract .app archive")
	}

	// verify the extracted .app exists
	if _, err := os.Stat(stagedAppDir); err != nil {
		// the archive may use a different .app name; find the first .app dir
		entries, dirErr := os.ReadDir(stagingDir)
		if dirErr != nil {
			return errors.Wrap(err, "staged .app not found after extraction")
		}
		stagedAppDir = ""
		for _, e := range entries {
			if e.IsDir() && strings.HasSuffix(e.Name(), ".app") {
				stagedAppDir = filepath.Join(stagingDir, e.Name())
				break
			}
		}
		if stagedAppDir == "" {
			return errors.New("no .app directory found in extracted archive")
		}
	}

	// Gatekeeper-gate the extracted bundle before advertising STAGED: a
	// tampered, unsigned, or truncated archive must not enter the swap path.
	// On non-darwin this is a no-op.
	if err := verifyAppBundleCodesign(ctx, stagedAppDir); err != nil {
		_ = os.RemoveAll(stagedAppDir)
		_ = removeStagedManifest(stagingDir)
		return errors.Wrap(err, "verify staged .app codesign")
	}

	// write the sidecar manifest so the next boot can verify that the
	// on-disk .app is still fresh against the latest DistConfig. Written
	// only after all signature gates passed.
	manifest := &spacewave_launcher.StagedManifest{
		Version:       version,
		Path:          stagedAppDir,
		SignatureHash: asset.GetSha256(),
	}
	if err := writeStagedManifest(stagingDir, manifest); err != nil {
		_ = os.RemoveAll(stagedAppDir)
		return errors.Wrap(err, "write staged manifest")
	}

	// set staged state
	c.modifyLauncherInfo(func(li *spacewave_launcher.LauncherInfo) (bool, error) {
		li.UpdateState = &spacewave_launcher.UpdateState{
			Phase:      spacewave_launcher.UpdatePhase_UpdatePhase_STAGED,
			Version:    version,
			StagedPath: stagedAppDir,
		}
		return true, nil
	})

	c.le.WithField("version", version).
		WithField("staged-path", stagedAppDir).
		Info(".app bundle update staged and ready")

	return nil
}

// extractTarGzWithXattrs extracts a tar.gz archive to destDir, preserving
// extended attributes from PAX headers. The archive should contain a single
// .app directory at the top level.
//
// Path safety: every write resolves its parent directory against the real
// filesystem and verifies the resolved path stays within destDir. Symlinks are
// only created when their target (after Clean and Join with the link's parent)
// also stays within destDir. That defeats both classic =../..= traversal and
// the symlink-mediated variant where an archive plants an in-tree symlink and
// then writes a regular file through it.
func extractTarGzWithXattrs(archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return errors.Wrap(err, "open archive")
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return errors.Wrap(err, "open gzip reader")
	}
	defer gz.Close()

	realDest, err := filepath.EvalSymlinks(destDir)
	if err != nil {
		return errors.Wrap(err, "resolve destination directory")
	}

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.Wrap(err, "read tar header")
		}

		// sanitize path to prevent directory traversal
		clean := filepath.Clean(hdr.Name)
		if strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
			continue
		}
		target := filepath.Join(destDir, clean)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := ensureSafeParent(realDest, target); err != nil {
				return errors.Wrap(err, "unsafe directory path "+clean)
			}
			if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
				return errors.Wrap(err, "create directory "+clean)
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errors.Wrap(err, "create parent dir for "+clean)
			}
			if err := ensureSafeParent(realDest, target); err != nil {
				return errors.Wrap(err, "unsafe file path "+clean)
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return errors.Wrap(err, "create file "+clean)
			}
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return errors.Wrap(err, "write file "+clean)
			}
			if err := out.Close(); err != nil {
				return errors.Wrap(err, "close file "+clean)
			}
		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return errors.Wrap(err, "create parent dir for symlink "+clean)
			}
			if err := ensureSafeParent(realDest, target); err != nil {
				return errors.Wrap(err, "unsafe symlink path "+clean)
			}
			if err := ensureSafeSymlinkTarget(realDest, target, hdr.Linkname); err != nil {
				return errors.Wrap(err, "unsafe symlink target "+clean)
			}
			_ = os.Remove(target)
			if err := os.Symlink(hdr.Linkname, target); err != nil {
				return errors.Wrap(err, "create symlink "+clean)
			}
		}

		// apply xattrs from PAX records
		if len(hdr.PAXRecords) > 0 {
			applyXattrsFromPAX(target, hdr.PAXRecords)
		}
	}
	return nil
}

// ensureSafeParent resolves target's parent directory via the filesystem and
// returns an error unless the real path is still within realDest. This guards
// against earlier-extracted symlinks in intermediate path components.
func ensureSafeParent(realDest, target string) error {
	parent := filepath.Dir(target)
	realParent, err := filepath.EvalSymlinks(parent)
	if err != nil {
		return errors.Wrap(err, "resolve parent directory")
	}
	return ensureWithin(realDest, realParent)
}

// ensureSafeSymlinkTarget rejects symlinks whose resolved target escapes
// realDest. The symlink target is evaluated as a filesystem path relative to
// the symlink's parent directory without following it.
func ensureSafeSymlinkTarget(realDest, linkPath, linkname string) error {
	if linkname == "" {
		return errors.New("empty symlink target")
	}
	if filepath.IsAbs(linkname) {
		return ensureWithin(realDest, filepath.Clean(linkname))
	}
	resolved := filepath.Clean(filepath.Join(filepath.Dir(linkPath), linkname))
	return ensureWithin(realDest, resolved)
}

// ensureWithin returns nil when child equals realDest or lies inside it.
func ensureWithin(realDest, child string) error {
	rel, err := filepath.Rel(realDest, child)
	if err != nil {
		return errors.Wrap(err, "relate path")
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.Errorf("path %q escapes destination %q", child, realDest)
	}
	return nil
}
