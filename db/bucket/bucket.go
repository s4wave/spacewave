// Package bucket defines a namespace of blocks inside a Volume and the
// configuration that governs it.
//
// A Bucket serves the block operations under one bucket ID. Its Config, whose
// Rev rises with each change, carries the default put options, such as the
// hash type, and the lookup configuration that decides where a read searches
// after the local stores miss. An ObjectRef addresses a graph by its root
// BlockRef, the bucket that holds it, and the transform configuration that
// decodes it. Empty fields inherit the parent's, so a graph can cross buckets
// and transforms at any ref.
//
// Package lookup builds the Cursor that follows ObjectRefs across buckets and
// the Lookup that a read goes through. Package lookup/concurrent searches
// every bucket handle at once and, configured to, asks the network through
// the dex.LookupBlockFromNetwork directive. A block it recovers is written
// back as a cache fill only when its outgoing refs are known, so a bucket
// never holds a block whose edges were lost.
package bucket

import (
	"github.com/s4wave/spacewave/db/block"
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
	// StoreOps implements the block store.
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
