package kvtx_block

import "github.com/pkg/errors"

// ErrUnknownImpl is returned if the implementation is unknown.
var ErrUnknownImpl = errors.New("unknown block k/v implementation")

// NewErrUnknownImpl constructs a new unknown implementation error
func NewErrUnknownImpl(impl KVImplType) error {
	return errors.Wrapf(ErrUnknownImpl, "%s", impl.String())
}
