package unixfs_access_http

import (
	"context"
	"net/http"
	"time"

	bifrost_http "github.com/aperturerobotics/bifrost/http"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_access "github.com/aperturerobotics/hydra/unixfs/access"
	"github.com/aperturerobotics/hydra/util/billyhttp"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
)

// NewHTTPHandlerBuilder constructs a HTTPHandlerBuilder function.
func NewHTTPHandlerBuilder(
	b bus.Bus,
	unixFsID, unixFsPrefix string,
	httpPrefix string,
	returnIfIdle bool,
) bifrost_http.HTTPHandlerBuilder {
	return func(ctx context.Context) (*http.Handler, func(), error) {
		val, valRef, err := unixfs_access.ExAccessUnixFS(ctx, b, unixFsID, returnIfIdle)
		if err != nil {
			return nil, nil, err
		}
		if valRef == nil {
			return nil, nil, nil
		}

		fsHandle, fsHandleRel, err := val(ctx)
		if err != nil {
			valRef.Release()
			return nil, nil, err
		}

		var billyfs billy.Filesystem = unixfs.NewBillyFS(ctx, fsHandle, "", time.Time{})
		if unixFsPrefix != "" && unixFsPrefix != "/" && unixFsPrefix != "." {
			billyfs = chroot.New(billyfs, unixFsPrefix)
		}
		hfs := billyhttp.NewFileSystem(billyfs, httpPrefix)
		handler := http.FileServer(hfs)
		return &handler, func() {
			fsHandleRel()
			valRef.Release()
		}, nil
	}
}
