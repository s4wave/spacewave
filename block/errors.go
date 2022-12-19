package block

import "errors"

var (
	// ErrBucketUnavailable is returned when Fetch is called against a nil bucket.
	ErrBucketUnavailable = errors.New("bucket is not set or is unavailable")
	// ErrUnexpectedType is returned if a type assertion failed.
	ErrUnexpectedType = errors.New("block: unexpected object type")
	// ErrNilCursor is returned when a non-nil block cursor is required.
	ErrNilCursor = errors.New("block cursor cannot be nil")
	// ErrNilBlock is returned when a non-nil block is required.
	ErrNilBlock = errors.New("block cannot be nil")
	// ErrNotBlock is returned if the block did not implement Block.
	ErrNotBlock = errors.New("block must be a block")
	// ErrNotSubBlock is returned if the block did not implement SubBlock.
	ErrNotSubBlock = errors.New("block must be a sub-block")
	// ErrNotBlockWithSubBlocks is returned if the block did not implement BlockWithSubBlocks.
	ErrNotBlockWithSubBlocks = errors.New("block must implement block with sub-blocks")
	// ErrEmptyChanges is returned if a slice of changes was unexpectedly empty.
	ErrEmptyChanges = errors.New("changes set cannot be empty")
	// ErrNotFound is returned when a block was not found but was required.
	ErrNotFound = errors.New("block not found")
	// ErrNotClonable is returned if a block could not be cloned.
	ErrNotClonable = errors.New("block: unable to clone")
	// ErrBlockRefMismatch is returned if the data does not match the expected ref.
	ErrBlockRefMismatch = errors.New("block: block ref hash mismatch")
)
