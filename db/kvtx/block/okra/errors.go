package kvtx_block_okra

import "github.com/pkg/errors"

var (
	// ErrUnexpectedEmptyRootHeight is returned when an empty Okra root carries height.
	ErrUnexpectedEmptyRootHeight = errors.New("okra empty root cannot have height")
	// ErrUnexpectedRootMetadata is returned when Okra root constants or refs are invalid.
	ErrUnexpectedRootMetadata = errors.New("okra root metadata is invalid")
	// ErrUnexpectedPageMetadata is returned when Okra page metadata is invalid.
	ErrUnexpectedPageMetadata = errors.New("okra page metadata is invalid")
	// ErrUnexpectedEntryMetadata is returned when Okra entry metadata is invalid.
	ErrUnexpectedEntryMetadata = errors.New("okra entry metadata is invalid")
	// ErrUnsortedEntries is returned when fixture entries are not strictly sorted.
	ErrUnsortedEntries = errors.New("okra entries must be strictly sorted")
)
