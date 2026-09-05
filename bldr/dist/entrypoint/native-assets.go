//go:build !js

package dist_entrypoint

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// nativeAssetsFS reads embedded assets, falling back to an executable-adjacent
// volume for distributions packaged with a sidecar. Callers close opened files.
type nativeAssetsFS struct {
	// FS contains the entrypoint's embedded files.
	fs.FS
	// executable resolves the installed executable independently of the cwd.
	executable func() (string, error)
}

// Open prefers the embedded volume. Only an absent embedded volume permits
// sidecar lookup, relative to the executable after resolving symlinks.
func (f nativeAssetsFS) Open(name string) (fs.File, error) {
	// Embedded distributions must not depend on or be shadowed by a sidecar.
	file, err := f.FS.Open(name)
	if name != "assets.kvfile" || !errors.Is(err, fs.ErrNotExist) {
		return file, err
	}

	// External volumes follow the executable when installed or moved.
	executable, err := f.executable()
	if err != nil {
		return nil, err
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return nil, err
	}
	return os.Open(filepath.Join(filepath.Dir(executable), name))
}

// _ is a type assertion
var _ fs.FS = nativeAssetsFS{}
