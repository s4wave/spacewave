package block

import (
	"context"

	"github.com/s4wave/spacewave/net/hash"
)

// NopStoreOps implements StoreOps defaults for tests.
//
// Production wrappers must not embed this type. Test mocks may embed it and
// override only the methods exercised by the test.
type NopStoreOps struct{}

// GetHashType returns the preferred hash type for the store.
func (NopStoreOps) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (NopStoreOps) GetSupportedFeatures() StoreFeature {
	return StoreFeature_STORE_FEATURE_UNKNOWN
}

// PutBlock returns ErrBlockStoreUnavailable.
func (NopStoreOps) PutBlock(context.Context, []byte, *PutOpts) (*BlockRef, bool, error) {
	return nil, false, ErrBlockStoreUnavailable
}

// PutBlockBatch loops calling PutBlock or RmBlock per entry.
func (n NopStoreOps) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) error {
	for _, entry := range entries {
		if entry.Tombstone {
			if err := n.RmBlock(ctx, entry.Ref); err != nil {
				return err
			}
			continue
		}
		var ref *BlockRef
		if entry.Ref != nil {
			ref = entry.Ref.Clone()
		}
		if _, _, err := n.PutBlock(ctx, entry.Data, &PutOpts{
			ForceBlockRef: ref,
			Refs:          entry.Refs,
		}); err != nil {
			return err
		}
	}
	return nil
}

// PutBlockBackground forwards to PutBlock.
func (n NopStoreOps) PutBlockBackground(ctx context.Context, data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	return n.PutBlock(ctx, data, opts)
}

// GetBlock returns a missing block.
func (NopStoreOps) GetBlock(context.Context, *BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

// GetBlockExists returns false.
func (NopStoreOps) GetBlockExists(context.Context, *BlockRef) (bool, error) {
	return false, nil
}

// GetBlockExistsBatch loops calling GetBlockExists per ref.
func (n NopStoreOps) GetBlockExistsBatch(ctx context.Context, refs []*BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		found, err := n.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
		out[i] = found
	}
	return out, nil
}

// RmBlock returns nil.
func (NopStoreOps) RmBlock(context.Context, *BlockRef) error {
	return nil
}

// StatBlock returns nil.
func (NopStoreOps) StatBlock(context.Context, *BlockRef) (*BlockStat, error) {
	return nil, nil
}

// Flush returns nil.
func (NopStoreOps) Flush(context.Context) error {
	return nil
}

// BeginDeferFlush opens a no-op defer-flush scope.
func (NopStoreOps) BeginDeferFlush() {}

// EndDeferFlush closes a no-op defer-flush scope.
func (NopStoreOps) EndDeferFlush(context.Context) error {
	return nil
}

// _ is a type assertion
var _ StoreOps = NopStoreOps{}
