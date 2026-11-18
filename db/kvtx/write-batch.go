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

// WriteBatchPrefetcher is an optional companion to WriteBatchTxOps for a
// transaction whose write batch reads stored pages before it changes them.
// PrefetchWriteBatch reads the pages ApplyWriteBatch of the keys would read,
// and changes nothing. A caller that holds writes for a later batch calls it
// when it holds them, so the batch, which may run where a read must not wait,
// finds its pages in memory. It is advisory: an error leaves the batch to read
// the page itself. Keys are borrowed only for the call.
type WriteBatchPrefetcher interface {
	PrefetchWriteBatch(ctx context.Context, keys [][]byte) error
}
