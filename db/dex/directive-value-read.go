package dex

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// ReadLookupBlockFromNetworkValue reads a block from store, with its refs when
// the store can supply them. Returns an empty value when the block is not
// found.
func ReadLookupBlockFromNetworkValue(ctx context.Context, store block.StoreOps, ref *block.BlockRef) (LookupBlockFromNetworkValue, error) {
	stored, err := store.GetStoredBlock(ctx, ref)
	switch {
	case err != nil:
		return nil, err
	case stored == nil:
		return NewLookupBlockFromNetworkValue(nil, nil), nil
	case stored.RefsKnown:
		return NewLookupBlockFromNetworkValueWithRefs(stored.Data, stored.Refs), nil
	default:
		return NewLookupBlockFromNetworkValue(stored.Data, nil), nil
	}
}
