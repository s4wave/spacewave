//go:build !js && !tinygo

package bldr

import (
	"context"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/s4wave/spacewave/bldr/distpath"
	util_iofs "github.com/s4wave/spacewave/bldr/util/iofs"
	"github.com/s4wave/spacewave/bldr/util/npm"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	"github.com/sirupsen/logrus"
)

// PrepareTypeScriptProject materializes the shipped compiler and SDK source closure.
// Bldr's dependency cache prepares both SDK and declared project dependencies
// before any compiler starts. The source and dist directories are owned checkouts.
func PrepareTypeScriptProject(ctx context.Context, le *logrus.Entry, sourceRoot, distRoot string, sources map[string]fs.FS) error {
	// Preserve package-relative imports by materializing the declared directory layout.
	for _, target := range slices.Sorted(maps.Keys(sources)) {
		if !fs.ValidPath(target) {
			return &fs.PathError{Op: "materialize sources", Path: target, Err: fs.ErrInvalid}
		}
		cursor, err := unixfs_iofs.NewFSCursor(util_iofs.NewWritableFS(sources[target]))
		if err != nil {
			return err
		}
		handle, err := unixfs.NewFSHandle(cursor)
		if err != nil {
			cursor.Release()
			return err
		}
		err = unixfs_sync.Sync(ctx, filepath.Join(distRoot, filepath.FromSlash(target)), handle,
			unixfs_sync.DeleteMode_DeleteMode_DURING, nil)
		handle.Release()
		if err != nil {
			return err
		}
	}

	// SDK files resolve their pinned packages independently of project dependencies.
	install, err := npm.EnsureSharedBunInstall(ctx, le, distRoot,
		distpath.Resolve(distRoot, "dist", "deps", "package.json"), filepath.Join(distRoot, "deps"))
	if err != nil {
		return err
	}
	if err := os.Symlink(filepath.Join(install, "node_modules"), filepath.Join(distRoot, "node_modules")); err != nil {
		return err
	}
	// Generated compiler services run under .bldr and use the SDK's dependencies.
	// The project's own install must not shadow the compiler's runtime packages.
	buildRoot := filepath.Join(sourceRoot, ".bldr")
	if err := os.MkdirAll(buildRoot, 0o755); err != nil {
		return err
	}
	if err := os.Symlink(filepath.Join(install, "node_modules"), filepath.Join(buildRoot, "node_modules")); err != nil {
		return err
	}

	projectPackage := filepath.Join(sourceRoot, "package.json")
	if _, err := os.Stat(projectPackage); err == nil {
		install, err = npm.EnsureSharedBunInstall(ctx, le, distRoot, projectPackage, filepath.Join(distRoot, "project-deps"))
		if err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Symlink(filepath.Join(install, "node_modules"), filepath.Join(sourceRoot, "node_modules")); err != nil {
		return err
	}
	return nil
}
