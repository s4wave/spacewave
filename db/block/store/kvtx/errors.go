package block_store_kvtx

import "github.com/pkg/errors"

var (
	// ErrReadOperationClosed is returned when a read scope is used after release.
	ErrReadOperationClosed = errors.New("kvtx block read operation closed")
	// ErrReadOperationReadOnly is returned when a read scope receives a write.
	ErrReadOperationReadOnly = errors.New("kvtx block read operation is read-only")
)
