package kvtx

import "context"

// WriteBatchEntry describes a value replacement or deletion. A nil Value is an
// empty value, not a deletion. Key and Value are borrowed only for the call.
type WriteBatchEntry struct {
	Key, Value []byte
	Delete     bool
}

// WriteBatchTxOps is an optional optimization for transactions that can coalesce
// tree mutations. It does not commit, change the durable format, or add a
// savepoint. The last entry for a key wins; discard the transaction on error.
// Implementations must own any input they retain after the call returns.
type WriteBatchTxOps interface {
	ApplyWriteBatch(ctx context.Context, entries []WriteBatchEntry) error
}
