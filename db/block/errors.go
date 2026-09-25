package block

import "errors"

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
	// ErrRefsUnknown is returned when a graph copy reaches a block whose source
	// holds its bytes without its outgoing refs.
	ErrRefsUnknown = errors.New("block refs unknown")
	// ErrNotClonable is returned if a block could not be cloned.
	ErrNotClonable = errors.New("block: unable to clone")
	// ErrBlockRefMismatch is returned if the data does not match the expected ref.
	ErrBlockRefMismatch = errors.New("block: block ref hash mismatch")
	// ErrBufferedStoreFull is returned when a buffered store reaches its memory limits.
	ErrBufferedStoreFull = errors.New("block: buffered store is full")
	// ErrAtomicPublicationUnsupported is returned before admission, with no side
	// effects, when a store cannot atomically persist blocks, ownership and metadata.
	// It is the only publication error for which a caller may use a legacy path.
	ErrAtomicPublicationUnsupported = errors.New("atomic publication unsupported")
	// ErrPublicationClosed means the publication writer no longer accepts work.
	ErrPublicationClosed = errors.New("publication writer closed")
	// ErrPublicationTooLarge rejects a publication that cannot fit the writer's
	// bounded admission budget. Large block bodies should be prepared durably first.
	ErrPublicationTooLarge = errors.New("publication exceeds bounded admission budget")
	// ErrPublicationDependency means a predecessor failed or was not submitted to
	// this writer before its dependent publication.
	ErrPublicationDependency = errors.New("publication predecessor did not succeed")
)

// publicationDependencyError joins the dependency sentinel with the predecessor
// failure. Callers match the sentinel with errors.Is while the original cause
// stays inspectable.
type publicationDependencyError struct {
	cause error
}

func (e *publicationDependencyError) Error() string {
	return ErrPublicationDependency.Error() + ": " + e.cause.Error()
}

// Unwrap returns both parents: the dependency sentinel and the predecessor result.
func (e *publicationDependencyError) Unwrap() []error {
	return []error{ErrPublicationDependency, e.cause}
}

// NewPublicationDependencyError wraps a failed or unordered predecessor result
// with ErrPublicationDependency. Use it to reject a dependent publication.
func NewPublicationDependencyError(cause error) error {
	return &publicationDependencyError{cause: cause}
}
