package bucket

import (
	"github.com/aperturerobotics/hydra/bucket/event"
	"github.com/aperturerobotics/hydra/cid"
)

// bucketRW combines a read and write bucket together.
type bucketRW struct {
	readHandle  Bucket
	writeHandle Bucket
}

// NewBucketRW constructs a new Bucket handle using a read handle and an
// optional write handle. If the write handle is not nil, the write (put and
// delete) calls will go to it. Otherwise, all calls are sent to the read
// handle.
func NewBucketRW(readHandle, writeHandle Bucket) Bucket {
	if writeHandle == nil {
		writeHandle = readHandle
	}
	return &bucketRW{
		readHandle:  readHandle,
		writeHandle: writeHandle,
	}
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (b *bucketRW) PutBlock(data []byte, opts *PutOpts) (*bucket_event.PutBlock, error) {
	return b.writeHandle.PutBlock(data, opts)
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (b *bucketRW) GetBlock(ref *cid.BlockRef) ([]byte, bool, error) {
	return b.readHandle.GetBlock(ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (b *bucketRW) RmBlock(ref *cid.BlockRef) error {
	return b.writeHandle.RmBlock(ref)
}

// _ is a type assertion
var _ Bucket = ((*bucketRW)(nil))
