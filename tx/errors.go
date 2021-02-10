package tx

import "errors"

var (
	// ErrDiscarded is returned if the transaction was already discarded or committed.
	ErrDiscarded = errors.New("transaction has already been discarded or committed")
	// ErrNotWrite is returned if Commit is called on a non-write transaction.
	ErrNotWrite = errors.New("commit called on non-write transaction")
)
