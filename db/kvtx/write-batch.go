package kvtx

import "context"

// WriteBatchEntry describes a value replacement or deletion. A nil Value is an
// empty value, not a deletion. Key and Value are borrowed only for the call.
type WriteBatchEntry struct {
	// Key is the key to replace or delete, borrowed for the call.
	Key []byte
	// Value is the replacement value, borrowed for the call. nil is empty.
	Value []byte
	// Delete removes Key instead of storing Value.
	Delete bool
}

// WriteBatchTxOps is an optional optimization for transactions that can coalesce
// tree mutations. It does not commit, change the durable format, or add a
// savepoint. The last entry for a key wins. Discard the transaction on error.
// Implementations must own any input they retain after the call returns.
type WriteBatchTxOps interface {
	// ApplyWriteBatch applies replacements and deletions without
	// committing. The last entry for each key wins. Discard the transaction
	// on error.
	ApplyWriteBatch(ctx context.Context, entries []WriteBatchEntry) error
}
