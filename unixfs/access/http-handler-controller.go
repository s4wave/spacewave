package unixfs_access

import (
	"context"
	"regexp"

	bifrost_http "github.com/aperturerobotics/bifrost/http"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
)

// HTTPHandlerController serves AccessUnixFS to LookupHTTPHandler directives.
type HTTPHandlerController = *bifrost_http.HTTPHandlerController

// NewHTTPHandlerController constructs a new HTTP handler controller.
//
// matchPathPrefixes is the list of path prefixes to match.
// stripPathPrefix strips the matchPathPrefix before calling the handler.
// pathRe is an optional regex to match the paths, can be nil.
// unixFsPrefix is an optional prefix path to apply to all FS lookups.
// httpPrefix is an optional path prefix to strip from HTTP requests.
// returnIfIdle returns 404 error if the AccessUnixFS becomes idle.
func NewHTTPHandlerController(
	ctx context.Context,
	b bus.Bus,
	info *controller.Info,
	matchPathPrefixes []string,
	stripPathPrefix bool,
	pathRe *regexp.Regexp,
	unixFsID,
	unixFsPrefix,
	httpPrefix string,
	returnIfIdle bool,
) *bifrost_http.HTTPHandlerController {
	return bifrost_http.NewHTTPHandlerController(
		info,
		NewHTTPHandler(
			ctx,
			b,
			unixFsID,
			unixFsPrefix,
			httpPrefix,
			returnIfIdle,
		),
		matchPathPrefixes,
		stripPathPrefix,
		pathRe,
	)
}
