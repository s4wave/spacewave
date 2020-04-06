package blob

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/cid"
	"github.com/gogo/protobuf/proto"
	"github.com/pkg/errors"
)

// NewBlobBlock builds a new blob root block.
func NewBlobBlock() block.Block {
	return &Blob{}
}

// Validate validates the blob type from known types.
func (b BlobType) Validate() error {
	switch b {
	case BlobType_BlobType_RAW:
	default:
		return errors.Errorf("unknown blob type: %s", b.String())
	}

	return nil
}

// FetchToBuffer fetches a full blob to a buffer.
// Note: the block cursor context is also used.
func FetchToBuffer(ctx context.Context, bcs *block.Cursor, buf *bytes.Buffer) error {
	rootBlock, err := bcs.Unmarshal(NewBlobBlock)
	if err != nil || rootBlock == nil {
		return err
	}
	root := rootBlock.(*Blob)
	if err := root.GetBlobType().Validate(); err != nil {
		return err
	}

	if root.GetTotalSize() == 0 {
		return nil
	}

	switch root.GetBlobType() {
	case BlobType_BlobType_RAW:
		if len(root.GetRawData()) != int(root.GetTotalSize()) {
			return errors.Errorf(
				"raw blob size mismatch: %d != actual %d",
				len(root.GetRawData()),
				root.GetTotalSize(),
			)
		}
		_, err := buf.Write(root.GetRawData())
		return err
	}

	return errors.Errorf("unimplemented blob type: %s", root.GetBlobType().String())
}

// FetchToBytes fetches to a bytes slice.
func FetchToBytes(ctx context.Context, bcs *block.Cursor) ([]byte, error) {
	var buf bytes.Buffer
	if err := FetchToBuffer(ctx, bcs, &buf); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// ValidateFull performs a full fetch and validate on the blob.
// Depending on the blob implementation this will fetch data.
// The block cursor should be located at the blob root.
func (b *Blob) ValidateFull(ctx context.Context, bcs *block.Cursor) error {
	if err := b.GetBlobType().Validate(); err != nil {
		return err
	}

	blobType := b.GetBlobType()
	totalSize := b.GetTotalSize()
	if totalSize == 0 {
		if blobType != BlobType_BlobType_RAW {
			return errors.New("empty blobs must be of raw type")
		}
		return nil
	}

	rdLen := len(b.GetRawData())
	if blobType == BlobType_BlobType_RAW {
		if len(b.GetRawData()) != int(b.GetTotalSize()) {
			return errors.Errorf(
				"raw blob size mismatch: %d != actual %d",
				len(b.GetRawData()),
				b.GetTotalSize(),
			)
		}
		return nil
	}

	if rdLen != 0 {
		return errors.New("non-raw blob type: raw data field should be empty")
	}

	/* TODO
	switch blobType {
	case BlobType_
	}
	*/

	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *Blob) MarshalBlock() ([]byte, error) {
	return proto.Marshal(b)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *Blob) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, b)
}

// ApplyBlockRef applies a ref change with a field id.
// The reference may be nil if the child block is nil.
func (b *Blob) ApplyBlockRef(id uint32, ptr *cid.BlockRef) error {
	// ref id is based on field number
	return nil
}

// GetBlockRefs returns all block references by ID.
// May return nil, and values may also be nil.
// Note: this does not include pending references (in a cursor)
func (b *Blob) GetBlockRefs() (map[uint32]*cid.BlockRef, error) {
	return nil, nil
}

// GetBlockRefCtor returns the constructor for the block at the ref id.
// Return nil to indicate invalid ref ID or unknown.
func (b *Blob) GetBlockRefCtor(id uint32) block.Ctor {
	return nil
}

// _ is a type assertion
var _ block.Block = ((*Blob)(nil))
