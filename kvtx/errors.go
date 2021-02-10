package kvtx

import "github.com/aperturerobotics/hydra/tx"

var (
	// ErrDiscarded is returned if the transaction was already discarded or committed.
	ErrDiscarded = tx.ErrDiscarded
	// ErrNotWrite is returned if Commit is called on a non-write transaction.
	ErrNotWrite = tx.ErrNotWrite
)
