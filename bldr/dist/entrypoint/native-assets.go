//go:build !js

package dist_entrypoint

import (
	"io/fs"
	"os"
	"path/filepath"
)

// nativeAssetsFS keeps configuration embedded and reads the distribution volume
// beside the executable. The caller owns each returned file's lifetime.
type nativeAssetsFS struct {
	fs.FS
	executable func() (string, error)
}

// Open resolves the volume relative to the actual executable, including when
// launched through a symlink. Other assets retain their embedded filesystem.
func (f nativeAssetsFS) Open(name string) (fs.File, error) {
	if name != "assets.kvfile" {
		return f.FS.Open(name)
	}
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
