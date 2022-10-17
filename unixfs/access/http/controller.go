package unixfs_access_http

import (
	"regexp"

	bifrost_http "github.com/aperturerobotics/bifrost/http"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/blang/semver"
)

// ControllerID is the controller identifier.
const ControllerID = "hydra/unixfs/access/http"

// Version is the controller version.
var Version = semver.MustParse("0.0.1")

// Controller serves AccessUnixFS to LookupHTTPHandler directives.
type Controller = bifrost_http.HTTPHandlerController

// NewController constructs a new HTTP handler controller.
//
// matchPathPrefixes is the list of path prefixes to match.
// stripPathPrefix strips the matchPathPrefix before calling the handler.
// pathRe is an optional regex to match the paths, can be nil.
// unixFsPrefix is an optional prefix path to apply to all FS lookups.
// httpPrefix is an optional path prefix to strip from HTTP requests.
// returnIfIdle returns 404 error if the AccessUnixFS becomes idle.
func NewController(
	b bus.Bus,
	info *controller.Info,
	matchPathPrefixes []string,
	stripPathPrefix bool,
	pathRe *regexp.Regexp,
	unixFsID,
	unixFsPrefix,
	httpPrefix string,
	returnIfIdle bool,
) *Controller {
	handlerBuilder := NewHTTPHandlerBuilder(b, unixFsID, unixFsPrefix, httpPrefix, returnIfIdle)
	return bifrost_http.NewHTTPHandlerController(
		info,
		handlerBuilder,
		matchPathPrefixes,
		stripPathPrefix,
		pathRe,
	)
}
