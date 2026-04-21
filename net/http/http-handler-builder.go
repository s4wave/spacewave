package bifrost_http

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/util/refcount"
)

// HTTPHandlerBuilder builds a HTTP Handle.
//
// returns the http handler and an optional release function
// can return nil to indicate not found.
//
// func(ctx context.Context, released func()) (http.Handler, func(), error)
type HTTPHandlerBuilder = refcount.RefCountResolver[http.Handler]

// NewHTTPHandlerBuilder creates a new HTTPHandlerBuilder with a static handler.
func NewHTTPHandlerBuilder(handler http.Handler) HTTPHandlerBuilder {
	return func(ctx context.Context, released func()) (http.Handler, func(), error) {
		if handler == nil {
			return nil, nil, nil
		}
		return handler, nil, nil
	}
}
