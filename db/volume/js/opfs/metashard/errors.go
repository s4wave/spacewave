//go:build js

package metashard

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
)

// ErrReadOnly is returned when a write operation is attempted on a read-only transaction.
var ErrReadOnly = errors.New("read-only transaction")

// ErrGenerationChanged is returned when another agent commits between two
// operations of the same transaction. Each operation reads one committed
// generation, so continuing would let the transaction assemble a view of the
// metadata that no generation ever held. The caller discards the transaction
// and retries.
var ErrGenerationChanged = errors.New("metadata committed by another agent during this transaction")

// CorruptError is returned when committed metashard state cannot be decoded.
type CorruptError struct {
	Err error
}

// NewCorruptError constructs a corrupt metashard error.
func NewCorruptError(err error) *CorruptError {
	return &CorruptError{Err: err}
}

// Error returns the corruption error message.
func (e *CorruptError) Error() string {
	if e.Err == nil {
		return "corrupt meta shard"
	}
	return errors.Wrap(e.Err, "corrupt meta shard").Error()
}

// Unwrap returns the underlying corruption error.
func (e *CorruptError) Unwrap() error {
	return e.Err
}

// IsCorruptError reports whether err indicates corrupt committed metashard state.
func IsCorruptError(err error) bool {
	var corruptErr *CorruptError
	return errors.As(err, &corruptErr) || pagestore.IsCorruptPageError(err)
}
