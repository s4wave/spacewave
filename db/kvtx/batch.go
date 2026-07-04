package kvtx

import "context"

// BatchTxOps reads multiple keys in one transaction operation.
type BatchTxOps interface {
	// GetBatch returns values and found flags aligned with keys.
	GetBatch(ctx context.Context, keys [][]byte) (values [][]byte, found []bool, err error)
}

// GetBatch reads multiple keys with the owner's batch path when available.
func GetBatch(ctx context.Context, ops TxOps, keys [][]byte) ([][]byte, []bool, error) {
	if batch, ok := ops.(BatchTxOps); ok {
		return batch.GetBatch(ctx, keys)
	}
	return GetBatchFallback(ctx, ops, keys)
}

// GetBatchFallback reads keys with the scalar transaction operation.
func GetBatchFallback(ctx context.Context, ops TxOps, keys [][]byte) ([][]byte, []bool, error) {
	values := make([][]byte, len(keys))
	found := make([]bool, len(keys))
	for i, key := range keys {
		if len(key) == 0 {
			return nil, nil, ErrEmptyKey
		}
		value, ok, err := ops.Get(ctx, key)
		if err != nil {
			return nil, nil, err
		}
		values[i] = value
		found[i] = ok
	}
	return values, found, nil
}
