package store

import "context"

// Transport fetches raw byte ranges from a remote packfile.
//
// Implementations are the lowest layer in the pack access pipeline: they do
// not cache, deduplicate, or verify anything. The engine wraps a transport in
// a span store (resident bytes), block catalog (block-level publication
// state), and publication queue (verify + writeback).
type Transport interface {
	// Fetch returns bytes [off, off+length) from the pack in a single call.
	// The returned slice has len <= length; a short read signals end-of-pack.
	Fetch(ctx context.Context, off int64, length int) ([]byte, error)
}
