package blob

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
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
	case BlobType_BlobType_CHUNKED:
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

// Validate performs cursory validation of the Blob object.
func (b *Blob) Validate() error {
	blobType := b.GetBlobType()
	if err := blobType.Validate(); err != nil {
		return errors.Wrap(err, "blob_type")
	}
	if blobType == BlobType_BlobType_RAW {
		if len(b.GetRawData()) != int(b.GetTotalSize()) {
			return ErrRawBlobSizeMismatch
		}
	} else if len(b.GetRawData()) != 0 {
		return errors.New("raw_data field must be empty for non-raw blob")
	}

	if blobType == BlobType_BlobType_CHUNKED {
		if err := b.GetChunkIndex().Validate(); err != nil {
			return err
		}
	} else {
		if len(b.GetChunkIndex().GetChunks()) != 0 {
			return errors.New("expected empty chunks field for non-chunked blob type")
		}
	}

	return nil
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

// ApplySubBlock applies a sub-block change with a field id.
func (b *Blob) ApplySubBlock(id uint32, next block.SubBlock) error {
	var ok bool
	switch id {
	case 4:
		b.ChunkIndex, ok = next.(*ChunkIndex)
		if !ok {
			return block.ErrUnexpectedType
		}
	}
	return nil
}

// GetSubBlocks returns all constructed sub-blocks by ID.
// May return nil, and values may also be nil.
func (b *Blob) GetSubBlocks() map[uint32]block.SubBlock {
	m := make(map[uint32]block.SubBlock)
	m[4] = b.GetChunkIndex()
	return m
}

// GetSubBlockCtor returns a function which creates or returns the existing
// sub-block at reference id. Can return nil to indicate invalid reference id.
func (b *Blob) GetSubBlockCtor(id uint32) block.SubBlockCtor {
	switch id {
	case 4:
		return func(create bool) block.SubBlock {
			v := b.GetChunkIndex()
			if v == nil && create {
				v = &ChunkIndex{}
				b.ChunkIndex = v
			}
			return v
		}
	}
	return nil
}

// _ is a type assertion
var (
	_ block.Block              = ((*Blob)(nil))
	_ block.BlockWithSubBlocks = ((*Blob)(nil))
)
