package provider_local

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

var (
	// ErrNoPackBackend is returned by LocateBlocks when no storage backend is
	// open for the store, as before its first placement opens.
	ErrNoPackBackend = errors.New("no storage backend is open for the block store")
	// ErrNoPackIndex is returned by LocateBlocks when the open backend keeps
	// no local index of its packs.
	ErrNoPackIndex = errors.New("the block store's backend keeps no pack index")
)

// blockLocator is a backend store that reports which blocks its packs hold
// from a local index of their keys.
type blockLocator interface {
	// LocateBlocks reports, for each ref, whether a pack holds it.
	LocateBlocks(ctx context.Context, refs []*block.BlockRef) ([]bool, error)
}

// LocateBlocks reports, for each ref, whether the open storage backend's local
// pack index places it in a pack. It reads the index alone: it reads no pack,
// never asks the read fallback or peers, and stores nothing. A pack the index
// has not covered yet answers false, so a false is a block to check again, not
// proof of absence. Each call is one read transaction on the index.
func (b *BlockStore) LocateBlocks(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Require an open backend whose store keeps a pack index.
	remote := b.placement.remote.Load()
	if remote == nil {
		return nil, ErrNoPackBackend
	}
	locator, ok := remote.store.(blockLocator)
	if !ok {
		return nil, ErrNoPackIndex
	}
	return locator.LocateBlocks(ctx, refs)
}
