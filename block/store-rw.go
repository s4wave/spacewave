package block

// StoreRW combines a read and write store together.
type StoreRW struct {
	readHandle  Store
	writeHandle Store
}

// NewStoreRW constructs a new Store handle using a read handle and an optional
// write handle. If the write handle is not nil, the write (put and delete)
// calls will go to it. Otherwise, all calls are sent to the read handle.
func NewStoreRW(readHandle, writeHandle Store) Store {
	if writeHandle == nil {
		writeHandle = readHandle
	}
	return &StoreRW{
		readHandle:  readHandle,
		writeHandle: writeHandle,
	}
}

// PutBlock puts a block into the store.
// The ref should not be modified after return.
func (b *StoreRW) PutBlock(data []byte, opts *PutOpts) (*BlockRef, bool, error) {
	return b.writeHandle.PutBlock(data, opts)
}

// GetBlock gets a block with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (b *StoreRW) GetBlock(ref *BlockRef) ([]byte, bool, error) {
	return b.readHandle.GetBlock(ref)
}

// GetBlockExists checks if a block exists with a cid reference.
// The ref should not be modified or retained by GetBlock.
// Note: the block may not be in the specified bucket.
func (b *StoreRW) GetBlockExists(ref *BlockRef) (bool, error) {
	return b.readHandle.GetBlockExists(ref)
}

// RmBlock deletes a block from the bucket.
// Does not return an error if the block was not present.
// In some cases, will return before confirming delete.
func (b *StoreRW) RmBlock(ref *BlockRef) error {
	return b.writeHandle.RmBlock(ref)
}

// _ is a type assertion
var _ Store = ((*StoreRW)(nil))
