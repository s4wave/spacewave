package volume

import "context"

// reserveExemptKey marks a context whose writes a volume admits below its
// disk reserve.
type reserveExemptKey struct{}

// WithReserveExempt marks ctx so that a volume admits its writes below the
// disk reserve. A delete writes the smaller state that replaces the one it
// removes, so it must write to free space; mark only such writes. The
// reserve's slack, not a separate bound, covers them.
func WithReserveExempt(ctx context.Context) context.Context {
	return context.WithValue(ctx, reserveExemptKey{}, true)
}

// ReserveExempt reports whether WithReserveExempt marked ctx.
func ReserveExempt(ctx context.Context) bool {
	exempt, _ := ctx.Value(reserveExemptKey{}).(bool)
	return exempt
}
