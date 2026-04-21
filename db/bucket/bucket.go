package bucket

import (
	"github.com/s4wave/spacewave/db/block"
	// "github.com/s4wave/spacewave/db/hash"
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
	// Store implements the block store.
	block.StoreOps
}

// BucketHandle is a bucket API handle.
// All calls use the bucket handle context.
type BucketHandle interface {
	// GetID returns the bucket ID.
	GetID() string
	// GetExists returns if the bucket exists. If false, the bucket does not
	// exist in the store, and all block calls will not work.
	GetExists() bool
	// GetBucketConfig returns the bucket configuration in use.
	// May be nil if the bucket does not exist in the store.
	GetBucketConfig() *Config
	// GetBucket returns the bucket object.
	// May be nil if the bucket does not exist in the store.
	GetBucket() Bucket
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
