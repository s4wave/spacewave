package bucket

import (
	bucket_event "github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
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
	// PutBlock puts a block into the store.
	// The ref should not be modified after return.
	PutBlock(data []byte, opts *PutOpts) (*bucket_event.PutBlock, error)
	// GetBlock gets a block with a cid reference.
	// The ref should not be modified or retained by GetBlock.
	// Note: the block may not be in the specified bucket.
	GetBlock(ref *cid.BlockRef) ([]byte, bool, error)
	// GetBlockExists checks if a block exists with a cid reference.
	// The ref should not be modified or retained by GetBlock.
	// Note: the block may not be in the specified bucket.
	GetBlockExists(ref *cid.BlockRef) (bool, error)
	// RmBlock deletes a block from the bucket.
	// Does not return an error if the block was not present.
	// In some cases, will return before confirming delete.
	RmBlock(ref *cid.BlockRef) error
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

// Validate validates the put opts.
func (o *PutOpts) Validate() error {
	if o == nil {
		return nil
	}
	if o.GetHashType() != 0 {
		if err := o.GetHashType().Validate(); err != nil {
			return err
		}
	}
	return nil
}
