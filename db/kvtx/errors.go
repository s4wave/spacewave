package kvtx

import (
	"errors"

	"github.com/s4wave/spacewave/db/tx"
)

var (
	// ErrDiscarded is returned if the transaction was already discarded or committed.
	ErrDiscarded = tx.ErrDiscarded
	// ErrNotWrite is returned if Commit is called on a non-write transaction.
	ErrNotWrite = tx.ErrNotWrite
	// ErrEmptyKey is returned if the key was empty.
	ErrEmptyKey = errors.New("key cannot be empty")
	// ErrBlockTxOpsUnimplemented is returned if the interface does not support BlockTxOps.
	ErrBlockTxOpsUnimplemented = errors.New("kvtx store does not implement block tx operations")
	// ErrKvtxSizeUnimplemented is returned if the store does not support Size.
	ErrKvtxSizeUnimplemented = errors.New("kvtx store does not support size lookup")
	// ErrNotFound is returned if the key was not found.
	ErrNotFound = errors.New("key was not found")
	// ErrInvalidSnapshot is returned when a transaction's storage snapshot can
	// no longer be trusted and the caller must reopen at a fresh generation.
	ErrInvalidSnapshot = errors.New("kvtx snapshot is invalid")
	// ErrTxTooBig is returned by a write when the transaction cannot hold it.
	// The write was not applied, and the transaction can still commit.
	ErrTxTooBig = errors.New("kvtx transaction is too big")
	// ErrWatchUnsupported is returned when a store cannot stream committed changes.
	ErrWatchUnsupported = errors.New("kvtx store does not support watch")
	// ErrWatchLimit is returned when a watched snapshot exceeds its limits.
	// No partial snapshot was delivered.
	ErrWatchLimit = errors.New("kvtx watch snapshot exceeds limits")
)
