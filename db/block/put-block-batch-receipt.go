package block

import "context"

// PutBlockBatchReceipt writes through the store's actual bound write path and
// returns its synchronous preparation result. A capable store reports its
// counting basis from that same physical transaction. A legacy store returns
// a completed unknown-basis receipt; existence alone cannot certify counts.
// Buffers remain borrowed only until this function returns. This is not head
// publication admission and its receipt is not a SubmitAtomic dependency.
func PutBlockBatchReceipt(ctx context.Context, store StoreOps, entries []*PutBatchEntry) (*PublicationReceipt, error) {
	// Preserve native evidence only from the actual bound write route.
	if prepared, ok := store.(interface {
		PutBlockBatchReceipt(context.Context, []*PutBatchEntry) (*PublicationReceipt, error)
	}); ok {
		return prepared.PutBlockBatchReceipt(ctx, entries)
	}

	// Legacy stores acknowledge bytes without certifying their count basis.
	existed, err := store.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	receipt := NewPublicationReceipt()
	receipt.ResolveExisted(nil, existed, false, 0)
	return receipt, nil
}
