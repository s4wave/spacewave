package block

import (
	"errors"
	"fmt"
)

var (
	// ErrBlockStoreUnavailable is returned when Fetch is called against a nil block store.
	ErrBlockStoreUnavailable = errors.New("block store is unavailable")
	// ErrUnexpectedType is returned if a type assertion failed.
	ErrUnexpectedType = errors.New("block: unexpected object type")
	// ErrNilCursor is returned when a non-nil block cursor is required.
	ErrNilCursor = errors.New("block cursor cannot be nil")
	// ErrNilBlock is returned when a non-nil block is required.
	ErrNilBlock = errors.New("block cannot be nil")
	// ErrEmptyBlock is returned when a non-empty block is required.
	ErrEmptyBlock = errors.New("block data cannot be nil")
	// ErrEmptyBlockRef is returned a ref was required but was empty.
	ErrEmptyBlockRef = errors.New("empty block reference")
	// ErrNotBlock is returned if the object did not implement Block.
	ErrNotBlock = errors.New("object must be a block")
	// ErrNotSubBlock is returned if the block did not implement SubBlock.
	ErrNotSubBlock = errors.New("block must be a sub-block")
	// ErrNotBlockWithSubBlocks is returned if the block did not implement BlockWithSubBlocks.
	ErrNotBlockWithSubBlocks = errors.New("block must implement block with sub-blocks")
	// ErrEmptyChanges is returned if a slice of changes was unexpectedly empty.
	ErrEmptyChanges = errors.New("changes set cannot be empty")
	// ErrNotFound is returned when a block was not found but was required.
	ErrNotFound = errors.New("block not found")
	// ErrUnavailable is returned when a read of a block that may exist failed:
	// no source served it, so a later read may succeed. Unlike ErrNotFound, it
	// does not report the block missing.
	ErrUnavailable = errors.New("block unavailable")
	// ErrRefsUnknown is returned when a graph copy reaches a block whose source
	// holds its bytes without its outgoing refs, and when a Volume refuses a
	// put that declares no child list.
	ErrRefsUnknown = errors.New("block refs unknown")
	// ErrSkipRefs is returned by a WalkGraph visit to leave the visited block's
	// refs unwalked. It is not an error.
	ErrSkipRefs = errors.New("skip block refs")
	// ErrReferenceCycle is returned when a write finds a block reachable from itself.
	ErrReferenceCycle = errors.New("block: reference cycle")
	// ErrNotClonable is returned if a block could not be cloned.
	ErrNotClonable = errors.New("block: unable to clone")
	// ErrBlockRefMismatch is returned if the data does not match the expected ref.
	ErrBlockRefMismatch = errors.New("block: block ref hash mismatch")
	// ErrBufferedStoreFull is returned when a buffered store reaches its memory limits.
	ErrBufferedStoreFull = errors.New("block: buffered store is full")
	// ErrAtomicCommitUnsupported is returned before any I/O, with no side
	// effects, when a store cannot commit blocks with their ownership in one
	// transaction.
	ErrAtomicCommitUnsupported = errors.New("atomic commit unsupported")
	// ErrPendingTombstone is returned by a PendingStore put of a tombstone: a
	// kept block may be named by any kept World, so none is deleted by a put.
	ErrPendingTombstone = errors.New("block: a pending store refuses tombstones")
	// ErrStageReleased rejects a write through a stage that was released or
	// reaped, which can no longer own the blocks.
	ErrStageReleased = errors.New("block: stage released")
)

// IsNotAvailable reports whether err means a block is not available here now:
// missing from the store, or not served by any source. A reader that needs the
// block can wait for it and retry, so the failure is not a property of the data
// that refers to it.
func IsNotAvailable(err error) bool {
	return errors.Is(err, ErrNotFound) || errors.Is(err, ErrUnavailable)
}

// NotFoundError returns ErrNotFound naming ref, so a log of the failure says
// which block is missing. errors.Is still matches ErrNotFound.
func NotFoundError(ref *BlockRef) error {
	return fmt.Errorf("%w: %s", ErrNotFound, ref.MarshalString())
}
