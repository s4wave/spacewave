package unixfs_access_http

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/bus"
	bifrost_http "github.com/s4wave/spacewave/net/http"
)

// HTTPHandler implements a HTTP handler which uses a refcount driven AccessUnixFS.
type HTTPHandler = bifrost_http.HTTPHandler

// NewHTTPHandler constructs a new HTTPHandler.
//
// NOTE: if ctx == nil the handler won't work until SetContext is called.
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
	handlerBuilder := NewHTTPHandlerBuilder(b, unixFsID, unixFsPrefix, httpPrefix, returnIfIdle)
	return bifrost_http.NewHTTPHandler(ctx, handlerBuilder)
}

// _ is a type assertion
var _ http.Handler = ((*HTTPHandler)(nil))
