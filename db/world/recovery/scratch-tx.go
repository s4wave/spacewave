//go:build !js && !wasip1

package world_recovery

import (
	"context"

	"github.com/s4wave/spacewave/db/kvtx"
)

// scratchTx retains indexed-operation errors until its callback returns.
type scratchTx struct {
	// tx supplies the native atomic transaction and snapshot semantics.
	tx kvtx.Tx
	// ctx bounds operations to this callback's lifetime.
	ctx context.Context
	// err keeps the first indexed read error, distinguishing failure from absence.
	err error
	// written counts the key and value bytes Put has written, to bound a batch.
	written int
}

// Bucket selects one index's disjoint prefix in the single scratch table.
func (tx *scratchTx) Bucket(name []byte) *scratchBucket {
	return &scratchBucket{tx: tx, prefix: append(append([]byte(nil), name...), 0)}
}

// fail preserves the first disk error so the enclosing transaction cannot commit.
func (tx *scratchTx) fail(err error) {
	if tx.err == nil {
		tx.err = err
	}
}
