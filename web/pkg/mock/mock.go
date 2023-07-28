package web_pkg_mock

import (
	"context"

	web_pkg "github.com/aperturerobotics/bldr/web/pkg"
	web_pkg_static "github.com/aperturerobotics/bldr/web/pkg/static"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	iofs_mock "github.com/aperturerobotics/hydra/unixfs/iofs/mock"
)

// MockWebPkgIDPrefix is the mock web package id prefix.
const MockWebPkgIDPrefix = "@aperturerobotics/"

// MockWebPkgID is the mock web package id.
const MockWebPkgID = MockWebPkgIDPrefix + "mock-package"

// MockWebPkgInfo is the mock web package information.
var MockWebPkgInfo = &web_pkg.WebPkgInfo{
	Id: MockWebPkgID,
}

// MockFS is the mock web package contents fs.
var MockFS, MockFSContents = iofs_mock.NewMockIoFS()

// NewMockWebPkg constructs the mock web package.
func NewMockWebPkg() web_pkg.WebPkg {
	staticPkg, _ := web_pkg_static.NewStaticWebPkg(
		MockWebPkgInfo,
		func(ctx context.Context) (*unixfs.FSHandle, error) {
			fsc, err := unixfs_iofs.NewFSCursor(MockFS)
			if err != nil {
				return nil, err
			}
			return unixfs.NewFSHandle(fsc)
		},
	)
	return staticPkg
}
