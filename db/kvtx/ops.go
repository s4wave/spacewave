package kvtx

import "context"

// MustGet performs Get against a kvtx store and returns ErrNotFound if not found.
func MustGet(ctx context.Context, o TxOps, key []byte) ([]byte, error) {
	val, found, err := o.Get(ctx, key)
	if err == nil && !found {
		err = ErrNotFound
	}
	if err != nil {
		val = nil
	}
	return val, err
}
