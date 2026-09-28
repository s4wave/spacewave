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

	"github.com/pkg/errors"
)

// StartCopiedProcess starts a verified, state-local copy of the desktop
// executable. The digest path remains available while that daemon is running.
func StartCopiedProcess(ctx context.Context, statePath, source string) error {
	executable, err := copyExecutable(statePath, source)
	if err != nil {
		return err
	}
	return StartExecutable(ctx, statePath, executable)
}

// copyExecutable publishes a digest-named executable without replacing an
// existing path, including one that a daemon may currently be executing.
func copyExecutable(statePath, source string) (published string, retErr error) {
	// Keep the daemon copy outside any application bundle, even with a state-root symlink.
	root, err := filepath.EvalSymlinks(statePath)
	if err != nil {
		return "", errors.Wrap(err, "resolve daemon state root")
	}
	for dir := root; dir != filepath.Dir(dir); dir = filepath.Dir(dir) {
		if strings.HasSuffix(strings.ToLower(filepath.Base(dir)), ".app") {
			return "", errors.Errorf("daemon state root is inside application bundle %s", dir)
		}
	}

	// Hash the bundled source before selecting its immutable destination name.
	input, err := os.Open(source)
	if err != nil {
		return "", errors.Wrap(err, "open bundled daemon executable")
	}
	defer input.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, input); err != nil {
		return "", errors.Wrap(err, "hash bundled daemon executable")
	}
	expected := digest.Sum(nil)

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
	path := filepath.Join(dir, hex.EncodeToString(expected)+filepath.Ext(source))

	// A previous launch's copy is reusable only after validating its bytes.
	present, err := verifyExecutable(path, expected)
	if err != nil {
		return "", err
	}
	if present {
		return path, nil
	}

	// Create a private temporary inode for the bundled executable.
	if _, err := input.Seek(0, io.SeekStart); err != nil {
		return "", errors.Wrap(err, "rewind bundled daemon executable")
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
		return "", errors.Wrap(err, "copy bundled daemon executable")
	}
	if !bytes.Equal(copyDigest.Sum(nil), expected) {
		_ = output.Close()
		return "", errors.New("bundled daemon executable changed while copying")
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
