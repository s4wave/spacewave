package kvtx

import (
	"bytes"
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// publicationOverlay is bounded by the submitted delta, never by volume size.
// Original batch slices are read-only; reads return independently owned bytes.
type publicationOverlay struct {
	// StoreOps reads the group's prior writes and the applied blocks.
	block.StoreOps
	// entries is the request's entry index by block key.
	entries map[string]*block.PutBatchEntry
}

// newPublicationOverlay builds a transaction-scoped read overlay serving a
// request's verified entry index before the inner store.
func newPublicationOverlay(inner block.StoreOps, entries map[string]*block.PutBatchEntry) *publicationOverlay {
	return &publicationOverlay{StoreOps: inner, entries: entries}
}

// BeginReadOperation returns the overlay itself as its read scope.
func (o *publicationOverlay) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return o, func() {}, nil
}

// GetBlock reads an entry from the overlay before the inner store. Every
// read is staged: the entries and the group transaction under them are not
// durable until the group commits, and the commit can still fail.
func (o *publicationOverlay) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	block.MarkStagedRead(ctx)
	key, err := ref.MarshalKey()
	if err != nil {
		return nil, false, err
	}
	if e := o.entries[string(key)]; e != nil {
		if e.Tombstone {
			return nil, false, nil
		}
		return bytes.Clone(e.Data), true, nil
	}
	return o.StoreOps.GetBlock(ctx, ref)
}

// GetBlockExists reports overlay tombstones before the inner store.
func (o *publicationOverlay) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	key, err := ref.MarshalKey()
	if err != nil {
		return false, err
	}
	if e := o.entries[string(key)]; e != nil {
		return !e.Tombstone, nil
	}
	return o.StoreOps.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch reports overlay visibility for each probed reference.
func (o *publicationOverlay) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	out := make([]bool, len(refs))
	for i, ref := range refs {
		var err error
		out[i], err = o.GetBlockExists(ctx, ref)
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// StatBlock reports overlay entry size before the inner store.
func (o *publicationOverlay) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	key, err := ref.MarshalKey()
	if err != nil {
		return nil, err
	}
	if e := o.entries[string(key)]; e != nil {
		if e.Tombstone {
			return nil, nil
		}
		return &block.BlockStat{Ref: ref.Clone(), Size: int64(len(e.Data))}, nil
	}
	return o.StoreOps.StatBlock(ctx, ref)
}

// PutBlock rejects mutation through a validation overlay.
func (o *publicationOverlay) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, false, errPublicationReadOnly
}

// PutBlockBatch rejects mutation through a validation overlay.
func (o *publicationOverlay) PutBlockBatch(context.Context, []*block.PutBatchEntry) ([]bool, error) {
	return nil, errPublicationReadOnly
}

// RmBlock rejects mutation through a validation overlay.
func (o *publicationOverlay) RmBlock(context.Context, *block.BlockRef) error {
	return errPublicationReadOnly
}

// Sync rejects the durability barrier through a validation overlay.
func (o *publicationOverlay) Sync(context.Context) (bool, error) {
	return false, errPublicationReadOnly
}
