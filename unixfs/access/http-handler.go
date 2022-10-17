package unixfs_access

import (
	"context"
	"net/http"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/util/ccontainer"
	"github.com/aperturerobotics/controllerbus/util/refcount"
	"github.com/aperturerobotics/hydra/unixfs"
	"github.com/aperturerobotics/hydra/util/billyhttp"
	"github.com/go-git/go-billy/v5"
	"github.com/go-git/go-billy/v5/helper/chroot"
)

// HTTPHandler implements a HTTP handler which uses a refcount driven AccessUnixFS.
type HTTPHandler struct {
	// b is the bus to send the AccessUnixFS directive.
	b bus.Bus
	// unixFsID is the filesystem ID to look up.
	unixFsID string
	// unixFsPrefix is an optional prefix path to apply to all FS lookups.
	// if empty, uses no prefix.
	unixFsPrefix string
	// httpPrefix is an optional prefix path to strip from HTTP requests.
	httpPrefix string
	// returnIfIdle returns 404 error if the AccessUnixFS becomes idle.
	returnIfIdle bool
	// handleCtr is the refcount handle to the UnixFS
	handleCtr *ccontainer.CContainer[*http.Handler]
	// errCtr contains any error building FSHandle
	errCtr *ccontainer.CContainer[*error]
	// rc is the refcount container
	rc *refcount.RefCount[*http.Handler]
}

// NewHTTPHandler constructs a new HTTPHandler.
//
// unixFsPrefix is an optional prefix path to apply to all FS lookups.
// httpPrefix is an optional path prefix to strip from HTTP requests.
// returnIfIdle returns 404 error if the AccessUnixFS becomes idle.
func NewHTTPHandler(
	ctx context.Context,
	b bus.Bus,
	unixFsID, unixFsPrefix string,
	httpPrefix string,
	returnIfIdle bool,
) *HTTPHandler {
	h := &HTTPHandler{
		b:            b,
		unixFsID:     unixFsID,
		unixFsPrefix: unixFsPrefix,
		httpPrefix:   httpPrefix,
		returnIfIdle: returnIfIdle,
		handleCtr:    ccontainer.NewCContainer[*http.Handler](nil),
		errCtr:       ccontainer.NewCContainer[*error](nil),
	}
	h.rc = refcount.NewRefCount(ctx, h.handleCtr, h.errCtr, h.resolveFSHandle)
	return h
}

// resolveFSHandle looks up the FSHandle.
func (h *HTTPHandler) resolveFSHandle(ctx context.Context) (*http.Handler, func(), error) {
	val, valRef, err := ExAccessUnixFS(ctx, h.b, h.unixFsID, h.returnIfIdle)
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
	if h.unixFsPrefix != "" && h.unixFsPrefix != "/" && h.unixFsPrefix != "." {
		billyfs = chroot.New(billyfs, h.unixFsPrefix)
	}
	hfs := billyhttp.NewFileSystem(billyfs, h.httpPrefix)
	handler := http.FileServer(hfs)
	return &handler, func() {
		fsHandleRel()
		valRef.Release()
	}, nil
}

// ServeHTTP serves a http request.
func (h *HTTPHandler) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	ctx := req.Context()
	err := refcount.AccessRefCount(ctx, h.rc, func(access *http.Handler) error {
		if access == nil {
			rw.WriteHeader(404)
			rw.Write([]byte("404 not found"))
			return nil
		}

		(*access).ServeHTTP(rw, req)
		return nil
	})
	if err != nil {
		rw.WriteHeader(500)
		rw.Write([]byte(err.Error()))
		return
	}
}

// _ is a type assertion
var _ http.Handler = ((*HTTPHandler)(nil))
