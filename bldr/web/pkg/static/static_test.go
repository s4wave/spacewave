package web_pkg_static

import (
	"context"
	"io"
	"testing"

	web_pkg "github.com/s4wave/spacewave/bldr/web/pkg"
	web_pkg_controller "github.com/s4wave/spacewave/bldr/web/pkg/controller"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/core"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	iofs_mock "github.com/s4wave/spacewave/db/unixfs/iofs/mock"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// TestStaticWebPkg tests the static web package.
func TestStaticWebPkg(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	webPkgID := "@aperturerobotics/test-package"
	info := &web_pkg.WebPkgInfo{
		Id: webPkgID,
	}

	mockFS, _ := iofs_mock.NewMockIoFS()
	staticWebPkg, err := NewStaticWebPkg(
		info,
		func(ctx context.Context) (*unixfs.FSHandle, error) {
			fsc, err := unixfs_iofs.NewFSCursor(mockFS)
			if err != nil {
				return nil, err
			}
			return unixfs.NewFSHandle(fsc)
		},
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	ctrl := web_pkg_controller.NewControllerWithWebPkg(
		le,
		controller.NewInfo("web/pkg/static/test", semver.MustParse("0.0.1"), "test web pkg"),
		staticWebPkg,
	)

	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	rel, err := b.AddController(ctx, ctrl, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer rel()

	pkg, _, relPkg, err := web_pkg.ExLookupWebPkg(ctx, b, true, webPkgID)
	if err != nil {
		t.Fatal(err.Error())
	}
	if relPkg != nil {
		defer relPkg.Release()
	}

	if pkg.GetId() != webPkgID {
		t.FailNow()
	}

	pkgInfo, err := pkg.GetInfo(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	if !pkgInfo.EqualVT(info) {
		t.FailNow()
	}

	fsHandle, err := pkg.GetWebPkgFsHandle(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	f, _, err := fsHandle.LookupPath(ctx, "testdir/testing.txt")
	if err != nil {
		t.Fatal(err.Error())
	}
	err = f.AccessOps(ctx, func(cursor unixfs.FSCursor, ops unixfs.FSCursorOps) error {
		data := make([]byte, 1024)
		n, err := ops.ReadAt(ctx, 0, data)
		if err != nil && err != io.EOF {
			return err
		}
		data = data[:n]
		dataStr := string(data)
		if dataStr != "file within a directory" {
			return errors.Errorf("unexpected data: %v", dataStr)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}
}
