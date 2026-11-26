package block

import "context"

// readAheadKey identifies the operation's preferred range-fetch minimum.
type readAheadKey struct{}

// WithReadAhead requests at least bytes per remote range-cache miss for this
// operation. Stores may limit the request to an uncovered gap, the end of a
// file, or their transport and memory bounds. Resident reads need no fetch.
// The hint does not change a shared store's default policy.
func WithReadAhead(ctx context.Context, bytes int) context.Context {
	return context.WithValue(ctx, readAheadKey{}, max(0, bytes))
}

// ReadAhead returns the requested range-fetch minimum, or zero when unset.
func ReadAhead(ctx context.Context) int {
	bytes, _ := ctx.Value(readAheadKey{}).(int)
	return bytes
}

// subtreeReadKey marks an operation that walks whole subtrees.
type subtreeReadKey struct{}

// WithSubtreeRead marks ctx as a walk that reads each block's subtree, such as
// a graph walk or a range scan, rather than one path through it. A store that
// knows where a block's subtree lies, such as a pack written in graph order,
// may read the subtree's structure with the block and keep it for the walk's
// next reads. A point lookup leaves the mark off, since it reads one child per
// level.
func WithSubtreeRead(ctx context.Context) context.Context {
	return context.WithValue(ctx, subtreeReadKey{}, true)
}

// IsSubtreeRead reports whether ctx carries WithSubtreeRead's mark.
func IsSubtreeRead(ctx context.Context) bool {
	on, _ := ctx.Value(subtreeReadKey{}).(bool)
	return on
}
