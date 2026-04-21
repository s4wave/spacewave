package web_pkg_fs_controller

import (
	"context"
	"io/fs"

	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_controller "github.com/s4wave/spacewave/bldr/web/pkg/controller"
	web_pkg_fs "github.com/s4wave/spacewave/bldr/web/pkg/fs"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// ControllerID is the controller identifier.
const ControllerID = "bldr/web/pkg/fs/controller"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

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
		val, valRef, err := unixfs_access.ExAccessUnixFS(ctx, b, unixFsID, returnIfIdle, released)
		if err != nil {
			return nil, nil, err
		}
		if valRef == nil {
			return nil, nil, errors.Wrap(unixfs_errors.ErrFsNotFound, unixFsID)
		}

		fsHandle, fsHandleRel, err := val(ctx, released)
		if err != nil {
			valRef.Release()
			return nil, nil, err
		}

		var childHandle *unixfs.FSHandle
		if unixFsPrefix != "" {
			childHandle, _, err = fsHandle.LookupPath(ctx, unixFsPrefix)
			if err != nil {
				fsHandleRel()
				valRef.Release()
				return nil, nil, err
			}
		}

		var ifs fs.FS
		if childHandle != nil {
			ifs = unixfs_iofs.NewFS(ctx, childHandle)
		} else {
			ifs = unixfs_iofs.NewFS(ctx, fsHandle)
		}

		pkg, pkgRel, err := web_pkg_fs.GetWebPkg(ctx, ifs, webPkgID)
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
