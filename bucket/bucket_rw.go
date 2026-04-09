package bucket

import (
	"context"

	"github.com/aperturerobotics/hydra/block"
)

// bucketRW combines a read and write bucket together.
type bucketRW struct {
	block.StoreOps
	conf *Config
}

// NewBucketRW constructs a new Bucket handle using a read handle and an
// optional write handle. If the write handle is not nil, the write (put and
// delete) calls will go to it. Otherwise, all calls are sent to the read
// handle.
func NewBucketRW(readHandle Bucket, writeHandle BucketOps) Bucket {
	if writeHandle == nil {
		writeHandle = readHandle
	}
	return &bucketRW{
		StoreOps: block.NewStoreRW(readHandle, writeHandle),
		conf:     readHandle.GetBucketConfig(),
	}
}

// GetBucketConfig returns a copy of the bucket configuration.
func (b *bucketRW) GetBucketConfig() *Config {
	return b.conf
}

// BeginDeferFlush forwards to the inner StoreOps if it supports deferred flushing.
func (b *bucketRW) BeginDeferFlush() {
	if df, ok := b.StoreOps.(block.DeferFlushable); ok {
		df.BeginDeferFlush()
	}
}

// EndDeferFlush forwards to the inner StoreOps if it supports deferred flushing.
func (b *bucketRW) EndDeferFlush(ctx context.Context) error {
	if df, ok := b.StoreOps.(block.DeferFlushable); ok {
		return df.EndDeferFlush(ctx)
	}
	return nil
}

// _ is a type assertion
var (
	_ Bucket               = ((*bucketRW)(nil))
	_ block.DeferFlushable = ((*bucketRW)(nil))
)
