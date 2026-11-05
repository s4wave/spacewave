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

// BeginReadOperation returns the no-op store for a read scope.
func (n NopStoreOps) BeginReadOperation(context.Context) (StoreOps, func(), error) {
	return n, func() {}, nil
}

// GetRoot reports that no name is held.
func (NopStoreOps) GetRoot(context.Context, string) (*BlockRef, error) { return nil, nil }

// ListRoots reports no roots.
func (NopStoreOps) ListRoots(context.Context, string, func(NamedRoot) error) error { return nil }

// SetRoots stores the entries and refuses a root that would hold a block:
// this store holds no roots.
func (n NopStoreOps) SetRoots(ctx context.Context, entries []*PutBatchEntry, roots []NamedRoot) error {
	return SetRootsUnheld(ctx, n, entries, roots)
}

// PutBlock returns ErrBlockStoreUnavailable.
func (NopStoreOps) PutBlock(context.Context, []byte, *PutOpts) (*BlockRef, bool, error) {
	return nil, false, ErrBlockStoreUnavailable
}

// PutBlockBatch loops calling PutBlock or RmBlock per entry.
func (n NopStoreOps) PutBlockBatch(ctx context.Context, entries []*PutBatchEntry) ([]bool, error) {
	existed := make([]bool, len(entries))
	for i, entry := range entries {
		if entry.Tombstone {
			if err := n.RmBlock(ctx, entry.Ref); err != nil {
				return nil, err
			}
			continue
		}
		var ref *BlockRef
		if entry.Ref != nil {
			ref = entry.Ref.Clone()
		}
		_, entryExisted, err := n.PutBlock(ctx, entry.Data, &PutOpts{
			ForceBlockRef: ref,
			Refs:          entry.Refs,
			RefsKnown:     entry.RefsKnown,
		})
		if err != nil {
			return nil, err
		}
		existed[i] = entryExisted
	}
	return existed, nil
}

// GetBlock returns a missing block.
func (NopStoreOps) GetBlock(context.Context, *BlockRef) ([]byte, bool, error) {
	return nil, false, nil
}

// GetStoredBlock returns a missing block.
func (NopStoreOps) GetStoredBlock(context.Context, *BlockRef) (*StoredBlock, error) {
	return nil, nil
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

// Sync reports no durability fence because the no-op store holds no writes.
func (NopStoreOps) Sync(context.Context) (bool, error) {
	return false, nil
}

// _ is a type assertion
var _ StoreOps = NopStoreOps{}
