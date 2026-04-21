package unixfs_access_http

import (
	"context"
	"net/http"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/helper/chroot"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_errors "github.com/s4wave/spacewave/db/unixfs/errors"
	"github.com/s4wave/spacewave/db/util/billyhttp"
	bifrost_http "github.com/s4wave/spacewave/net/http"
)

// NewHTTPHandlerBuilder constructs a HTTPHandlerBuilder function.
//
// if returnIfIdle is set and the directive becomes idle, returns ErrFsNotFound
func NewHTTPHandlerBuilder(
	b bus.Bus,
	unixFsID, unixFsPrefix string,
	httpPrefix string,
	returnIfIdle bool,
) bifrost_http.HTTPHandlerBuilder {
	return func(ctx context.Context, released func()) (http.Handler, func(), error) {
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

		hfs := NewFileSystem(ctx, fsHandle, unixFsPrefix, httpPrefix)
		handler := NewFileServer(hfs)
		return handler, func() {
			fsHandleRel()
			valRef.Release()
		}, nil
	}
}

// NewFileSystem constructs a new http.FileSystem from a fsHandle.
func NewFileSystem(
	ctx context.Context,
	fsHandle *unixfs.FSHandle,
	unixFsPrefix, httpPrefix string,
) http.FileSystem {
	var billyfs billy.Filesystem = unixfs_billy.NewBillyFS(ctx, fsHandle, "", time.Time{})
	if unixFsPrefix != "" && unixFsPrefix != "/" && unixFsPrefix != "." {
		billyfs = chroot.New(billyfs, unixFsPrefix)
	}
	return billyhttp.NewFileSystem(billyfs, httpPrefix)
}

// NewFileServer builds a new http.FileServer which has extended content-type support.
func NewFileServer(hfs http.FileSystem) http.Handler {
	return http.FileServer(hfs)
}
