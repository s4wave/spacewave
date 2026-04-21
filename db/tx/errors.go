package tx

import "errors"

var (
	// ErrDiscarded is returned if the transaction was already discarded or committed.
	ErrDiscarded = errors.New("transaction has already been discarded or committed")
	// ErrNotWrite is returned if the transaction is read only.
	ErrNotWrite = errors.New("transaction is read-only")
)
