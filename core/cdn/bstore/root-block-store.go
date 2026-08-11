package cdn_bstore

import (
	"context"

	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/core/cdn"
)

// RootBlockStore reads a CDN Space and tracks its root pointer.
//
// The pointer carries the SORoot a reader builds its world head from,
// so every mount of a CDN Space needs one whether or not that mount owns the
// CDN transport. CdnBlockStore owns transport, pack readers, and writeback;
// SuppliedBlockStore reads through an owner that already holds them.
type RootBlockStore interface {
	// BlockStore reads blocks for the CDN Space.
	bstore.BlockStore

	// Pointer returns the currently-cached root pointer without
	// triggering a fetch. Returns nil if no pointer has been fetched yet.
	Pointer() *cdn.CdnRootPointer
	// Refresh re-fetches the root pointer.
	// Returns nil if the CDN Space has no published root.
	Refresh(ctx context.Context) (*cdn.CdnRootPointer, error)
	// Close releases resources owned by the store.
	Close()
}

// _ is a type assertion
var _ RootBlockStore = (*CdnBlockStore)(nil)
