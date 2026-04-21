package bifrost_http

import (
	"context"
	"net/http"
	"regexp"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
)

// HTTPHandlerController resolves LookupHTTPHandler with a http.Handler.
type HTTPHandlerController struct {
	// info is the controller info
	info *controller.Info
	// handleCtr contains the http handler
	handleCtr *ccontainer.CContainer[http.Handler]
	// errCtr contains any error building the handler
	errCtr *ccontainer.CContainer[*error]
	// rc is the refcount container
	rc *refcount.RefCount[http.Handler]
	// pathPrefixes is the list of URL path prefixes to match.
	// ignores if empty
	pathPrefixes []string
	// stripPathPrefix removes the first matched pathPrefix from the URL.
	stripPathPrefix bool
	// pathRe is a regex to match URL paths.
	// ignores if empty
	pathRe *regexp.Regexp
}

// NewHTTPHandlerController constructs a new controller.
//
// Responds if a URL matches either pathPrefixes OR pathRe.
// pathPrefixes and pathRe can be empty.
// if stripPathPrefix is set, removes the pathPrefix from the URL.
func NewHTTPHandlerController(
	info *controller.Info,
	resolver HTTPHandlerBuilder,
	pathPrefixes []string,
	stripPathPrefix bool,
	pathRe *regexp.Regexp,
) *HTTPHandlerController {
	h := &HTTPHandlerController{
		info:            info,
		handleCtr:       ccontainer.NewCContainer[http.Handler](nil),
		errCtr:          ccontainer.NewCContainer[*error](nil),
		pathPrefixes:    pathPrefixes,
		stripPathPrefix: stripPathPrefix,
		pathRe:          pathRe,
	}
	h.rc = refcount.NewRefCount(nil, false, h.handleCtr, h.errCtr, resolver)
	return h
}

// GetControllerInfo returns information about the controller.
func (c *HTTPHandlerController) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// Execute executes the controller.
func (c *HTTPHandlerController) Execute(ctx context.Context) error {
	c.rc.SetContext(ctx)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *HTTPHandlerController) HandleDirective(
	ctx context.Context,
	inst directive.Instance,
) ([]directive.Resolver, error) {
	switch d := inst.GetDirective().(type) {
	case LookupHTTPHandler:
		rpath := d.LookupHTTPHandlerURL().Path
		// if we have no filters, match all.
		matched := len(c.pathPrefixes) == 0 && c.pathRe == nil
		var stripPrefix string
		if !matched && len(c.pathPrefixes) != 0 {
			for _, prefix := range c.pathPrefixes {
				if strings.HasPrefix(rpath, prefix) {
					matched = true
					stripPrefix = prefix
					break
				}
			}
		}
		if !matched && c.pathRe != nil {
			matched = c.pathRe.MatchString(rpath)
		}
		if !matched {
			return nil, nil
		}
		return directive.R(
			directive.NewRefCountResolverWithXfrm(
				c.rc,
				func(ctx context.Context, val http.Handler) (directive.Value, error) {
					if val == nil {
						return nil, nil
					}
					var handler LookupHTTPHandlerValue = val //nolint:staticcheck
					if c.stripPathPrefix && len(stripPrefix) != 0 {
						handler = http.StripPrefix(stripPrefix, handler)
					}
					return handler, nil
				},
			),
			nil,
		)
	}
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *HTTPHandlerController) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*HTTPHandlerController)(nil))
