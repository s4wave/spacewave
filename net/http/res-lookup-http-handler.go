package bifrost_http

import (
	"context"
	"net/http"

	"github.com/aperturerobotics/controllerbus/directive"
)

// LookupHTTPHandlerResolver resolves LookupHTTPHandler with a handler.
type LookupHTTPHandlerResolver struct {
	handler http.Handler
}

// NewLookupHTTPHandlerResolver constructs a new resolver.
func NewLookupHTTPHandlerResolver(handler http.Handler) *LookupHTTPHandlerResolver {
	return &LookupHTTPHandlerResolver{handler: handler}
}

// Resolve resolves the values, emitting them to the handler.
func (r *LookupHTTPHandlerResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	if hh := r.handler; hh != nil {
		_, _ = handler.AddValue(hh)
	}
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*LookupHTTPHandlerResolver)(nil))
