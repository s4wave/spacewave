package bldr_http

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupHTTPHandlerResolver resolves LookupHTTPHandler with a handler slice.
type LookupHTTPHandlerResolver struct {
	handlers []http.Handler
}

// NewLookupHTTPHandlerResolver constructs a new resolver.
func NewLookupHTTPHandlerResolver(handlers []http.Handler) *LookupHTTPHandlerResolver {
	return &LookupHTTPHandlerResolver{handlers: handlers}
}

// Resolve resolves the values, emitting them to the handler.
func (r *LookupHTTPHandlerResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	for _, hh := range r.handlers {
		if hh != nil {
			_, _ = handler.AddValue(hh)
		}
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*LookupHTTPHandlerResolver)(nil))
