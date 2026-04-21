package kvtx_block_iavl

import "errors"

var (
	// ErrMustBeBlock is returned if a cursor is not a block
	ErrMustBeBlock = errors.New("iavl value sub-block must implement block interface")
	// ErrUnexpectedBlob is returned if the "is-blob" flag was set when it shouldn't be.
	ErrUnexpectedBlob = errors.New("unexpected value ref blob flag in non-leaf node")
	// ErrUnexpectedValueRef is returned if the value ref was set when it shouldn't be.
	ErrUnexpectedValueRef = errors.New("unexpected value ref in non-leaf node")
)
