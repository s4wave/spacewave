package kvtx

import (
	"errors"

	"github.com/aperturerobotics/hydra/tx"
)

var (
	// ErrDiscarded is returned if the transaction was already discarded or committed.
	ErrDiscarded = tx.ErrDiscarded
	// ErrNotWrite is returned if Commit is called on a non-write transaction.
	ErrNotWrite = tx.ErrNotWrite
	// ErrEmptyKey is returned if the key was empty.
	ErrEmptyKey = errors.New("key cannot be empty")
	// ErrEmptyValue is returned if the value was empty.
	ErrEmptyValue = errors.New("value cannot be empty")
	// ErrBlockTxOpsUnimplemented is returned if the interface does not support BlockTxOps.
	ErrBlockTxOpsUnimplemented = errors.New("kvtx store does not implement block tx operations")
	// ErrKvtxSizeUnimplemented is returned if the store does not support Size.
	ErrKvtxSizeUnimplemented = errors.New("kvtx store does not support size lookup")
	// ErrNotFound is returned if the key was not found.
	ErrNotFound = errors.New("key was not found")
)
