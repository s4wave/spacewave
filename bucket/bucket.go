package bucket

import (
	"github.com/aperturerobotics/hydra/block"
	// "github.com/aperturerobotics/hydra/hash"
)

// Bucket is a bucket API handle.
// All calls use the bucket handle context.
type Bucket interface {
	// BucketOps indicates Bucket implements the bucket operations.
	BucketOps
	// GetBucketConfig returns a copy of the bucket configuration.
	GetBucketConfig() *Config
}

// BucketOps are operations against a bucket API handle.
// All calls use the bucket handle context.
type BucketOps interface {
	// Store implements the block store operations.
	block.Store
}

// NewBucketInfo constructs a new bucket info with required fields.
func NewBucketInfo(conf *Config) *BucketInfo {
	if conf == nil {
		return nil
	}

	return &BucketInfo{
		Config: conf,
	}
}

// Validate validates the op arguments.
func (r *BucketOpArgs) Validate() error {
	if r.GetBucketId() == "" {
		return ErrBucketIDEmpty
	}
	return nil
}
