package web_pkg_fs

import (
	"context"
	"io/fs"

	"github.com/pkg/errors"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_static "github.com/s4wave/spacewave/bldr/web/pkg/static"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
)

// GetWebPkg wraps ifs in a WebPkgGetter for the package identified by webPkgID.
// It returns nil, nil, nil when webPkgID does not exist.
func GetWebPkg(ctx context.Context, ifs fs.FS, webPkgID string) (web_pkg.LookupWebPkgValue, func(), error) {
	// Find the package directory and treat a missing package as an empty result.
	fi, err := fs.Stat(ifs, webPkgID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	// Reject paths that identify files instead of package directories.
	if !fi.IsDir() {
		return nil, nil, errors.Errorf("web pkg path is not a dir: %s", webPkgID)
	}

	// Restrict filesystem access to the selected package directory.
	subFS, err := fs.Sub(ifs, webPkgID)
	if err != nil {
		return nil, nil, err
	}

	// Create a cursor for traversing the package filesystem.
	fsCursor, err := unixfs_iofs.NewFSCursor(subFS)
	if err != nil {
		return nil, nil, err
	}

	// Wrap the cursor in a filesystem handle with an explicit release function.
	fsHandle, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		return nil, nil, err
	}

	// Build the static web package from the releasable filesystem handle.
	spkg, err := web_pkg_static.NewStaticWebPkg(
		&web_pkg.WebPkgInfo{Id: webPkgID},
		fsHandle.Clone,
	)
	if err != nil {
		return nil, nil, err
	}

	return spkg, fsHandle.Release, nil
}
