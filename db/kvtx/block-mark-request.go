package kvtx

import (
	"context"
	"sync/atomic"
)

// BlockMarkRequest asks the transaction that stores a write's blocks to mark
// them. It tells the caller whether any transaction did.
type BlockMarkRequest struct {
	// marker records the blocks.
	marker BlockMarker
	// applied is set once a transaction marked blocks for this request.
	applied atomic.Bool
}

// blockMarkContextKey keys the BlockMarkRequest of a context.
type blockMarkContextKey struct{}

// WithBlockMarker returns a context whose block writes are marked by marker,
// in the transactions that store them, and the request that reports whether
// they were. Use one request per write call.
//
// A store that cannot mark in its transaction leaves the request unapplied, so
// the caller can fail the write instead of acknowledging unmarked blocks.
func WithBlockMarker(ctx context.Context, marker BlockMarker) (context.Context, *BlockMarkRequest) {
	req := &BlockMarkRequest{marker: marker}
	return WithBlockMarkRequest(ctx, req), req
}

// WithBlockMarkRequest returns a context that carries req, or none when req is
// nil. Work that runs on behalf of several callers, such as a queue drain,
// uses it to give each write its own caller's request.
func WithBlockMarkRequest(ctx context.Context, req *BlockMarkRequest) context.Context {
	return context.WithValue(ctx, blockMarkContextKey{}, req)
}

// GetBlockMarkRequest returns the request ctx carries, or nil for none.
func GetBlockMarkRequest(ctx context.Context) *BlockMarkRequest {
	req, _ := ctx.Value(blockMarkContextKey{}).(*BlockMarkRequest)
	return req
}

// GetMarker returns the marker that records the blocks.
func (r *BlockMarkRequest) GetMarker() BlockMarker {
	return r.marker
}

// Applied reports whether a transaction marked blocks for the request.
func (r *BlockMarkRequest) Applied() bool {
	return r.applied.Load()
}

// SetApplied records that a transaction marked blocks for the request.
func (r *BlockMarkRequest) SetApplied() {
	r.applied.Store(true)
}
