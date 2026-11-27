//go:build !js

package harness

import "context"

// Shape identifies content, looks up validated publications, and builds artifacts.
type Shape[A any] interface {
	// ContentKey returns the immutable content key to resolve.
	ContentKey(context.Context) (string, error)
	// Lookup returns every validated publication for the content key in
	// preference order. The first publication is the preferred result.
	Lookup(context.Context, string) ([]Generation[A], error)
	// Build produces and publishes one generation for the content key.
	Build(context.Context, string) (Generation[A], error)
}
