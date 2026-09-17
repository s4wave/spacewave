package kvtx_prefixer

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// writeBatchTx forwards write batches to the underlying transaction, prefixing
// each key. It exists only when the wrapped transaction supports batches.
type writeBatchTx struct {
	*tx
	// batch is the underlying transaction's write-batch capability.
	batch kvtx.WriteBatchTxOps
}

// ApplyWriteBatch prefixes each key and forwards the batch to the underlying
// transaction. It rejects empty keys before mutating anything.
func (t *writeBatchTx) ApplyWriteBatch(ctx context.Context, entries []kvtx.WriteBatchEntry) error {
	prefixed := make([]kvtx.WriteBatchEntry, len(entries))
	for i, entry := range entries {
		if len(entry.Key) == 0 {
			return kvtx.ErrEmptyKey
		}
		prefixed[i] = entry
		prefixed[i].Key = t.getKey(entry.Key)
	}
	return t.batch.ApplyWriteBatch(ctx, prefixed)
}
