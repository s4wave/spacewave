//go:build windows

package launcher_helper

import (
	stderrors "errors"
	"io"
	"os"

	"github.com/pkg/errors"
)

// prepareHelperExecutable copies a packaged helper to the user-writable
// application support directory. Windows blocks direct CreateProcess calls for
// undeclared executables inside an MSIX install directory even when the file's
// ACL grants read and execute access.
func prepareHelperExecutable(rootDir, helperPath string) (string, func() error, error) {
	content, err := os.ReadFile(helperPath)
	if err != nil {
		return "", nil, errors.Wrap(err, "read helper executable")
	}

	dst, err := os.CreateTemp(rootDir, "spacewave-helper-*.exe")
	if err != nil {
		return "", nil, errors.Wrap(err, "create helper executable")
	}
	dstPath := dst.Name()
	cleanup := func() error {
		if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
			return errors.Wrap(err, "remove helper executable")
		}
		return nil
	}
	fail := func(cause error) (string, func() error, error) {
		return "", nil, stderrors.Join(cause, errors.Wrap(dst.Close(), "close helper executable"), cleanup())
	}

	n, err := dst.Write(content)
	if err != nil {
		return fail(errors.Wrap(err, "copy helper executable"))
	}
	if n != len(content) {
		return fail(io.ErrShortWrite)
	}
	if err := dst.Close(); err != nil {
		return "", nil, stderrors.Join(errors.Wrap(err, "close helper executable"), cleanup())
	}
	if err := os.Chmod(dstPath, 0o700); err != nil {
		return "", nil, stderrors.Join(errors.Wrap(err, "make helper executable"), cleanup())
	}
	return dstPath, cleanup, nil
}
