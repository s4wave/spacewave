package block

// Store can read/write blocks.
type Store interface {
	// PutBlock puts a block into the store.
	// The ref should not be modified after return.
	// The second return value can optionally indicate if the block already existed.
	PutBlock(data []byte, opts *PutOpts) (*BlockRef, bool, error)
	// GetBlock gets a block with a cid reference.
	// The ref should not be modified or retained by GetBlock.
	// Note: the block may not be in the specified bucket.
	GetBlock(ref *BlockRef) ([]byte, bool, error)
	// GetBlockExists checks if a block exists with a cid reference.
	// The ref should not be modified or retained by GetBlock.
	// Note: the block may not be in the specified bucket.
	GetBlockExists(ref *BlockRef) (bool, error)
	// RmBlock deletes a block from the bucket.
	// Does not return an error if the block was not present.
	// In some cases, will return before confirming delete.
	RmBlock(ref *BlockRef) error
}

// PutBlock marshals & puts a block into a bucket.
func PutBlock(bk Store, b Block) (*BlockRef, bool, error) {
	dat, err := b.MarshalBlock()
	if err != nil {
		return nil, false, err
	}
	return bk.PutBlock(dat, nil)
}
