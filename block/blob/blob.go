package blob

import (
	"bytes"
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
	"github.com/restic/chunker"
)

// NewBlobBlock builds a new blob root block.
func NewBlobBlock() block.Block {
	return &Blob{}
}

// UnmarshalBlob unmarshals the Blob block.
// Returns nil, nil if empty
func UnmarshalBlob(bcs *block.Cursor) (*Blob, error) {
	exi, err := bcs.Unmarshal(NewBlobBlock)
	if err != nil {
		return nil, err
	}
	if exi == nil {
		return nil, nil
	}
	ex, ok := exi.(*Blob)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return ex, nil
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
	default:
		rdr, err := NewReader(ctx, bcs)
		if err != nil {
			return err
		}
		defer rdr.Close()

		_, err = io.Copy(buf, rdr)
		if err != nil {
			return err
		}
		return nil
	}
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
	if b.GetTotalSize() == 0 {
		if blobType != 0 {
			return errors.Errorf("expected zero blob-type for empty blob: %s", blobType.String())
		}
	} else {
		if err := blobType.Validate(); err != nil {
			return errors.Wrap(err, "blob_type")
		}
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

// IsEmpty checks if the blob total size is zero.
func (b *Blob) IsEmpty() bool {
	return b.GetTotalSize() == 0
}

// WriteChunkIndex builds and writes the chunk index to the blob.
// bcs must be located at the blob.
func (b *Blob) WriteChunkIndex(ctx context.Context, bcs *block.Cursor, opts *BuildBlobOpts, rdr io.Reader) error {
	chkIdxBcs := bcs.FollowSubBlock(4)
	nChunkIndex, nTotalSize, err := BuildChunkIndex(
		ctx,
		rdr,
		chkIdxBcs,
		chunker.Pol(opts.GetChunkingPol()),
		opts.GetChunkingMinSize(), opts.GetChunkingMaxSize(),
	)
	if err != nil {
		return err
	}
	b.BlobType = BlobType_BlobType_CHUNKED
	b.ChunkIndex, b.TotalSize = nChunkIndex, nTotalSize
	return nil
}

// AppendData appends data to an existing blob.
func (b *Blob) AppendData(
	ctx context.Context,
	dataLen int64,
	rdr io.Reader,
	bcs *block.Cursor,
	opts *BuildBlobOpts,
) error {
	hwm := opts.GetRawHighWaterMark()
	if hwm == 0 {
		hwm = rawHighWaterMark
	}

	oldLen := b.GetTotalSize()
	nextLen := oldLen + uint64(dataLen)
	if b.GetBlobType() == BlobType_BlobType_RAW {
		if nextLen <= hwm {
			// append to existing raw data blob
			ndata := make([]byte, nextLen)
			_, err := io.ReadAtLeast(rdr, ndata[oldLen:], int(dataLen))
			if err != nil {
				return err
			}
			copy(ndata[:oldLen], b.GetRawData())
			b.RawData = ndata
			b.TotalSize = nextLen
		} else {
			// create a new chunked blob
			mrdr := io.MultiReader(
				bytes.NewReader(b.GetRawData()),
				io.LimitReader(rdr, dataLen),
			)
			err := b.WriteChunkIndex(ctx, bcs, opts, mrdr)
			if err != nil {
				return err
			}
			b.RawData = nil
		}
		bcs.SetBlock(b, true)
		return nil
	}

	if b.GetBlobType() != BlobType_BlobType_CHUNKED {
		return errors.Errorf("cannot extend blob type: %s", b.GetBlobType().String())
	}
	if b.ChunkIndex == nil {
		b.ChunkIndex = &ChunkIndex{}
	}

	// append to existing chunked blob
	chkIdxBcs := bcs.FollowSubBlock(4)
	chunks := b.GetChunkIndex().GetChunks()
	if len(chunks) == 0 {
		return b.WriteChunkIndex(ctx, bcs, opts, io.LimitReader(rdr, dataLen))
	}

	chunksSet := NewChunkSet(&chunks, chkIdxBcs.FollowSubBlock(1))

	// remove the last chunk
	lastChunkIdx := len(chunks) - 1
	lastChunk := chunks[lastChunkIdx]
	_, lastChunkBcs := chunksSet.Get(lastChunkIdx)
	chunksSet.GetCursor().ClearRef(uint32(lastChunkIdx))
	chunks = chunks[:lastChunkIdx]
	b.ChunkIndex.Chunks = chunks

	// fetch last chunk data
	lastChunkData, err := lastChunk.FetchData(lastChunkBcs, false)
	if err != nil {
		return err
	}

	// build new chunk index with last chunk + new data
	pol := chunker.Pol(opts.GetChunkingPol())
	chkIdx, totalSize, err := BuildChunkIndex(
		ctx,
		io.MultiReader(bytes.NewReader(lastChunkData), io.LimitReader(rdr, dataLen)),
		chkIdxBcs,
		pol,
		opts.GetChunkingMinSize(),
		opts.GetChunkingMaxSize(),
	)
	if err != nil {
		return err
	}
	b.ChunkIndex, b.TotalSize = chkIdx, totalSize
	bcs.MarkDirty()
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
