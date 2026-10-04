//go:build !js

package daemon

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/launcher/appbundle"
)

// StartCopiedProcess starts a verified, state-local copy of the desktop
// executable. The digest path remains available while that daemon is running.
func StartCopiedProcess(ctx context.Context, statePath, source string) error {
	executable, err := PrepareExecutable(statePath, source)
	if err != nil {
		return err
	}
	return StartExecutable(ctx, statePath, executable, LauncherDesktop)
}

// PrepareExecutable publishes a digest-named executable without replacing an
// existing path, including one that a daemon may currently be executing. The
// caller selects and verifies the source before preparing it for startup.
func PrepareExecutable(statePath, source string) (published string, retErr error) {
	return prepareExecutable(statePath, source, "")
}

// PrepareVerifiedExecutable publishes the selected executable only when its
// bytes match the digest accepted by the launcher controller.
func PrepareVerifiedExecutable(statePath, source, expectedSHA256 string) (string, error) {
	if len(expectedSHA256) != 64 {
		return "", errors.New("accepted daemon executable digest is missing")
	}
	return prepareExecutable(statePath, source, expectedSHA256)
}

// prepareExecutable copies selected bytes, with their application bundle when
// they have one, to a digest path without replacing a version that another
// daemon may still be executing.
func prepareExecutable(statePath, source, expectedSHA256 string) (published string, retErr error) {
	// Keep the state root outside any bundle an app update replaces, even through a symlink.
	root, err := filepath.EvalSymlinks(statePath)
	if err != nil {
		return "", errors.Wrap(err, "resolve daemon state root")
	}
	for dir := root; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if strings.HasSuffix(strings.ToLower(filepath.Base(dir)), ".app") {
			return "", errors.Errorf("daemon state root is inside application bundle %s", dir)
		}
	}

	// Hash the selected source before choosing its immutable destination name.
	input, err := os.Open(source)
	if err != nil {
		return "", errors.Wrap(err, "open selected daemon executable")
	}
	defer input.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, input); err != nil {
		return "", errors.Wrap(err, "hash selected daemon executable")
	}
	expected := digest.Sum(nil)
	if expectedSHA256 != "" && hex.EncodeToString(expected) != expectedSHA256 {
		return "", errors.New("selected daemon executable fails accepted digest")
	}

	// Keep digest versions in an owned directory outside the application bundle.
	dir := filepath.Join(root, "daemon-bin")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", errors.Wrap(err, "create daemon executable directory")
	}
	dirInfo, err := os.Lstat(dir)
	if err != nil {
		return "", errors.Wrap(err, "inspect daemon executable directory")
	}
	if !dirInfo.IsDir() {
		return "", errors.Errorf("daemon executable directory is not a directory: %s", dir)
	}
	name := hex.EncodeToString(expected)

	// macOS binds a bundle executable's signature to its Info.plist and kills a
	// lone copy at exec, so a bundled source is published with its whole bundle.
	if isBundle, bundleRoot := appbundle.Detect(source); isBundle {
		return prepareBundle(dir, name, bundleRoot, filepath.Base(source), expected)
	}
	path := filepath.Join(dir, name+filepath.Ext(source))

	// A previous launch's copy is reusable only after validating its bytes.
	present, err := verifyExecutable(path, expected)
	if err != nil {
		return "", err
	}
	if present {
		return path, nil
	}

	// Create a private temporary inode for the selected executable.
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return "", errors.Wrap(err, "rewind selected daemon executable")
	}
	output, err := os.CreateTemp(dir, ".daemon-*")
	if err != nil {
		return "", errors.Wrap(err, "create daemon executable copy")
	}
	defer func() {
		if err := os.Remove(output.Name()); err != nil && retErr == nil {
			published = ""
			retErr = errors.Wrap(err, "remove temporary daemon executable")
		}
	}()

	// Reject a source that changed between hashing and copying.
	copyDigest := sha256.New()
	if _, err := io.Copy(io.MultiWriter(output, copyDigest), input); err != nil {
		_ = output.Close()
		return "", errors.Wrap(err, "copy selected daemon executable")
	}
	if !bytes.Equal(copyDigest.Sum(nil), expected) {
		_ = output.Close()
		return "", errors.New("selected daemon executable changed while copying")
	}

	// Finish the file's executable mode and contents before publication.
	if err := output.Chmod(0o700); err != nil {
		_ = output.Close()
		return "", errors.Wrap(err, "make daemon copy executable")
	}
	if err := output.Sync(); err != nil {
		_ = output.Close()
		return "", errors.Wrap(err, "sync daemon executable copy")
	}
	if err := output.Close(); err != nil {
		return "", errors.Wrap(err, "close daemon executable copy")
	}

	// A hard link atomically publishes this inode only if the digest name is free.
	if err := os.Link(output.Name(), path); err != nil && !errors.Is(err, os.ErrExist) {
		return "", errors.Wrap(err, "publish daemon executable copy")
	}
	present, err = verifyExecutable(path, expected)
	if err != nil {
		return "", err
	}
	if !present {
		return "", errors.New("published daemon executable copy disappeared")
	}
	return path, nil
}

// prepareBundle publishes a copy of bundleRoot at dir/<name>.app and returns
// its executable. The rename publishes a fully verified bundle atomically and
// never replaces a bundle that another daemon may be executing.
func prepareBundle(dir, name, bundleRoot, executable string, expected []byte) (published string, retErr error) {
	// Name the published bundle and its executable path.
	bundle := filepath.Join(dir, name+".app")
	path := filepath.Join(bundle, "Contents", "MacOS", executable)

	// A previous launch's bundle is reusable only after validating its executable.
	present, err := verifyExecutable(path, expected)
	if err != nil {
		return "", err
	}
	if present {
		return path, nil
	}

	// Copy the bundle into a private temporary directory beside its destination.
	staging, err := os.MkdirTemp(dir, ".daemon-*")
	if err != nil {
		return "", errors.Wrap(err, "create daemon bundle copy")
	}
	defer func() {
		if err := os.RemoveAll(staging); err != nil && retErr == nil {
			published = ""
			retErr = errors.Wrap(err, "remove temporary daemon bundle")
		}
	}()
	copied := filepath.Join(staging, filepath.Base(bundleRoot))
	if err := os.CopyFS(copied, os.DirFS(bundleRoot)); err != nil {
		return "", errors.Wrap(err, "copy daemon application bundle")
	}

	// Reject a bundle whose executable changed between hashing and copying.
	present, err = verifyExecutable(filepath.Join(copied, "Contents", "MacOS", executable), expected)
	if err != nil {
		return "", errors.Wrap(err, "verify copied daemon bundle")
	}
	if !present {
		return "", errors.New("copied daemon bundle has no executable")
	}

	// Renaming onto an existing bundle fails, which keeps a published version intact.
	if err := os.Rename(copied, bundle); err != nil && !errors.Is(err, os.ErrExist) && !errors.Is(err, syscall.ENOTEMPTY) {
		return "", errors.Wrap(err, "publish daemon bundle copy")
	}
	present, err = verifyExecutable(path, expected)
	if err != nil {
		return "", err
	}
	if !present {
		return "", errors.New("published daemon bundle copy disappeared")
	}
	return path, nil
}

// verifyExecutable checks an existing digest path without following a symlink.
func verifyExecutable(path string, expected []byte) (bool, error) {
	// Reject links, nonfiles, and copies whose execute bit was removed.
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, errors.Wrap(err, "inspect daemon executable copy")
	}
	if !info.Mode().IsRegular() {
		return true, errors.Errorf("daemon executable copy is not a regular file: %s", path)
	}
	if info.Mode()&0o100 == 0 {
		return true, errors.Errorf("daemon executable copy is not executable: %s", path)
	}

	// Check the entire stored file against the current bundled source digest.
	input, err := os.Open(path)
	if err != nil {
		return true, errors.Wrap(err, "open daemon executable copy")
	}
	defer input.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, input); err != nil {
		return true, errors.Wrap(err, "hash daemon executable copy")
	}
	if !bytes.Equal(digest.Sum(nil), expected) {
		return true, errors.Errorf("daemon executable copy fails source digest: %s", path)
	}
	return true, nil
}
