package web_pkg_fs

import (
	"context"
	"io/fs"

	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_static "github.com/aperturerobotics/bldr/web/pkg/static"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/pkg/errors"
)

// GetWebPkg wraps an io/fs to provide a WebPkgGetter.
//
// Expects the following filesystem structure:
//
// - {web-pkg-id}/{filepath}
// - @myorg/mypkg/foo.js
// - react-dom/client.js
//
// Looks up directories with Stat within the root.
// GetWebPkg is the web package getter.
// Returns nil, nil nil if not found.
func GetWebPkg(ctx context.Context, ifs fs.FS, webPkgID string) (web_pkg.LookupWebPkgValue, func(), error) {
	fi, err := fs.Stat(ifs, webPkgID)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, nil
		}
		return nil, nil, err
	}

	if !fi.IsDir() {
		return nil, nil, errors.Errorf("web pkg path is not a dir: %s", webPkgID)
	}

	subFS, err := fs.Sub(ifs, webPkgID)
	if err != nil {
		return nil, nil, err
	}

	fsCursor, err := unixfs_iofs.NewFSCursor(subFS)
	if err != nil {
		return nil, nil, err
	}

	fsHandle, err := unixfs.NewFSHandle(fsCursor)
	if err != nil {
		return nil, nil, err
	}

	spkg, err := web_pkg_static.NewStaticWebPkg(
		&web_pkg.WebPkgInfo{Id: webPkgID},
		fsHandle.Clone,
	)
	if err != nil {
		return nil, nil, err
	}

	return spkg, fsHandle.Release, nil
}
