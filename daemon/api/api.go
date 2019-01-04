package api

import (
	"github.com/pkg/errors"
)

// Validate validates the operation code.
// Unknown is considered valid.
func (op BucketOp) Validate() error {
	switch op {
	case BucketOp_BucketOp_UNKNOWN:
	case BucketOp_BucketOp_BLOCK_GET:
	case BucketOp_BucketOp_BLOCK_PUT:
	case BucketOp_BucketOp_BLOCK_RM:
	default:
		return errors.Errorf("bucket op unknown: %v", op.String())
	}

	return nil
}

// Validate validates the request.
func (r *BucketOpRequest) Validate() error {
	if err := r.GetOp().Validate(); err != nil {
		return err
	}
	switch r.GetOp() {
	case BucketOp_BucketOp_BLOCK_RM:
		fallthrough
	case BucketOp_BucketOp_BLOCK_GET:
		if err := r.GetBlockRef().Validate(); err != nil {
			return errors.New("block ref must be specified")
		}
	case BucketOp_BucketOp_BLOCK_PUT:
		if len(r.GetData()) == 0 {
			return errors.New("empty block not allowed")
		}
		if err := r.GetPutOpts().Validate(); err != nil {
			return err
		}
	}
	return nil
}
