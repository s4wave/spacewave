//go:build !bldr_startup_trace || tinygo

package manifest_fetch_world

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
)

// Resolve resolves the values, emitting them to the handler.
func (r *fetchManifestResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	return r.resolve(ctx, handler)
}
