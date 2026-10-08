package web_pkg_fs_controller

import (
	"context"
	"io/fs"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_controller "github.com/s4wave/spacewave/bldr/web/pkg/controller"
	web_pkg_fs "github.com/s4wave/spacewave/bldr/web/pkg/fs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller identifier.
const ControllerID = "bldr/web/pkg/fs/controller"

// Version is the controller version.
var Version = controller.MustParseVersion("0.0.1")

// Controller uses AccessUnixFS to resolve LookupWebPkg directives.
type Controller = web_pkg_controller.Controller

// NewController constructs a new web pkg fs controller.
func NewController(
	le *logrus.Entry,
	b bus.Bus,
	cc *Config,
) (*Controller, error) {
	return web_pkg_controller.NewController(
		le,
		controller.NewInfo(ControllerID, Version, "web pkg fs controller"),
		NewWebPkgGetter(b, cc.GetUnixfsId(), cc.GetUnixfsPrefix(), cc.GetNotFoundIfIdle()),
		cc.GetWebPkgIdList(),
	), nil
}

// NewWebPkgGetter constructs a new web pkg getter function.
func NewWebPkgGetter(b bus.Bus, unixFsID, unixFsPrefix string, returnIfIdle bool) web_pkg_controller.WebPkgGetter {
	return func(ctx context.Context, webPkgID string, released func()) (web_pkg.LookupWebPkgValue, func(), error) {
		// Pin module URLs to the manifest when the filesystem serves an immutable one.
		assetBasePath, err := pinnedAssetBasePath(unixFsID, unixFsPrefix, webPkgID)
		if err != nil {
			return nil, nil, err
		}

		// Resolve the UnixFS directive that supplies the package filesystem.
		val, valRef, err := unixfs_access.ExAccessUnixFS(ctx, b, unixFsID, returnIfIdle, released)
		if err != nil {
			return nil, nil, err
		}
		if valRef == nil {
			return nil, nil, errors.Wrap(unixfs_errors.ErrFsNotFound, unixFsID)
		}

		// Acquire a filesystem handle while retaining the UnixFS directive.
		fsHandle, fsHandleRel, err := val(ctx, released)
		if err != nil {
			valRef.Release()
			return nil, nil, err
		}

		// Resolve the configured package prefix within the filesystem.
		var childHandle *unixfs.FSHandle
		if unixFsPrefix != "" {
			childHandle, _, err = fsHandle.LookupPath(ctx, unixFsPrefix)
			if err != nil {
				fsHandleRel()
				valRef.Release()
				return nil, nil, err
			}
		}

		// Adapt the selected filesystem root for Web package lookup.
		var ifs fs.FS
		if childHandle != nil {
			ifs = unixfs_iofs.NewFS(ctx, childHandle)
		} else {
			ifs = unixfs_iofs.NewFS(ctx, fsHandle)
		}

		// Load the Web package and release filesystem references if it is absent.
		pkg, pkgRel, err := web_pkg_fs.GetWebPkg(ctx, ifs, webPkgID, assetBasePath)
		if err != nil || pkg == nil {
			if childHandle != nil {
				childHandle.Release()
			}
			fsHandleRel()
			valRef.Release()
			return nil, nil, err
		}

		return pkg, func() {
			pkgRel()
			if childHandle != nil {
				childHandle.Release()
			}
			fsHandleRel()
			valRef.Release()
		}, nil
	}
}

// pinnedAssetBasePath returns the immutable URL prefix that serves webPkgID, or
// empty when unixFsID is not a manifest-bound plugin assets filesystem.
func pinnedAssetBasePath(unixFsID, unixFsPrefix, webPkgID string) (string, error) {
	// Only plugin assets filesystems have an immutable URL.
	artifactID, ok := strings.CutPrefix(unixFsID, bldr_plugin.PluginAssetsFsIdPrefix)
	if !ok {
		return "", nil
	}

	// An unpinned plugin binding serves its files directly.
	_, manifestRoot, err := bldr_plugin.ParsePluginArtifactID(artifactID, false)
	if err != nil || manifestRoot == "" {
		return "", err
	}
	return bldr_plugin.PluginAssetsHttpPrefix + artifactID + "/" + strings.Trim(unixFsPrefix, "/") + "/" + webPkgID + "/", nil
}
