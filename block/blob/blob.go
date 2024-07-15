package blob

import (
	"bytes"
	"context"
	"io"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/byteslice"
	"github.com/pkg/errors"
)

// NewBlobBlock builds a new blob root block.
func NewBlobBlock() block.Block {
	return &Blob{}
}

// NewBlobSubBlockCtor returns the sub-block constructor.
func NewBlobSubBlockCtor(r **Blob) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if create && v == nil {
			v = &Blob{}
			*r = v
		}
		return v
	}
}

// UnmarshalBlob unmarshals the Blob block.
// Returns nil, nil if empty
func UnmarshalBlob(ctx context.Context, bcs *block.Cursor) (*Blob, error) {
	return block.UnmarshalBlock[*Blob](ctx, bcs, NewBlobBlock)
}

// Validate validates the blob type from known types.
func (b BlobType) Validate() error {
	switch b {
	case BlobType_BlobType_RAW:
	case BlobType_BlobType_CHUNKED:
	default:
		return errors.Wrap(ErrUnknownBlobType, b.String())
	}

	return nil
}

// FetchToBuffer fetches a full blob to a buffer.
// Note: the block cursor context is also used.
func FetchToBuffer(ctx context.Context, bcs *block.Cursor, buf *bytes.Buffer) error {
	root, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		return err
	}
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

// IsEmpty checks if the blob total size is zero.
func (b *Blob) IsEmpty() bool {
	return b.GetTotalSize() == 0
}

// IsNil checks if the object is nil.
func (b *Blob) IsNil() bool {
	return b == nil
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

// ComputeStorageSize computes the total size of all blocks making up the Blob.
//
// note: not accurate until the btx has been committed.
// returns:
//   - storageSize: actual size of blocks on disk
//   - totalSize: size of blocks on disk ignoring duplicates (for dedupe comparison)
//   - err: any error
func (b *Blob) ComputeStorageSize(
	ctx context.Context,
	bcs *block.Cursor,
) (uint64, uint64, error) {
	var storageSize uint64

	// add the size of the root block
	rootData, _, err := bcs.Fetch(ctx)
	if err != nil {
		return 0, 0, err
	}
	storageSize += uint64(len(rootData))

	if b.GetBlobType() != BlobType_BlobType_CHUNKED {
		return storageSize, storageSize, nil
	}

	// if chunked, add the size of each chunk
	// assume raw chunks (avoid fetching them)
	totalSize := storageSize
	seenBlocks := make(map[string]struct{})
	for _, chunk := range b.GetChunkIndex().GetChunks() {
		blobSize := chunk.GetSize()
		totalSize += blobSize

		dataRef := chunk.GetDataRef()
		dataRefStr := dataRef.MarshalString()
		if dataRefStr == "" {
			continue
		}
		if _, ok := seenBlocks[dataRefStr]; ok {
			continue
		}
		seenBlocks[dataRefStr] = struct{}{}
		// assume storage block size == chunk size
		storageSize += blobSize
	}
	return storageSize, totalSize, nil
}

// ValidateFull performs a full fetch and validate on the blob.
// Depending on the blob implementation this will fetch data.
// The block cursor should be located at the blob root.
func (b *Blob) ValidateFull(ctx context.Context, bcs *block.Cursor) error {
	if err := b.GetBlobType().Validate(); err != nil {
		return err
	}

	blobType := b.GetBlobType()
	totalSize := int64(b.GetTotalSize())
	if totalSize == 0 {
		if blobType != BlobType_BlobType_RAW {
			return errors.New("empty blobs must be of raw type")
		}
		return nil
	}

	rdLen := len(b.GetRawData())
	if blobType == BlobType_BlobType_RAW {
		if len(b.GetRawData()) != int(totalSize) {
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

	// if we have no block cursor, skip remaining checks.
	if bcs == nil {
		return nil
	}

	// fetch all of the chunked data w/o errors
	rdr, err := NewReader(ctx, bcs)
	if err != nil {
		return err
	}
	defer rdr.Close()

	buf := make([]byte, 4096)
	var readn int64
	for readn < totalSize {
		rn, err := rdr.Read(buf)
		if err != nil && err != io.EOF {
			return err
		}
		// expect to read exactly totalSize
		if rn == 0 {
			return errors.Errorf("blob: eof before end of blob: %d < expected %d", readn, totalSize)
		}
		readn += int64(rn)
		if readn > totalSize {
			return errors.Errorf("blob: read past expected end: %d > expected %d", readn, totalSize)
		}
	}
	return nil
}

// WriteChunkIndex builds and writes the chunk index to the blob.
// bcs must be located at the blob.
func (b *Blob) WriteChunkIndex(ctx context.Context, bcs *block.Cursor, opts *BuildBlobOpts, rdr io.Reader) error {
	chkIdxBcs := bcs.FollowSubBlock(4)
	nChunkIndex, nTotalSize, err := BuildChunkIndex(
		ctx,
		rdr,
		chkIdxBcs,
		opts.GetChunkerArgs(),
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
		hwm = DefRawHighWaterMark
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

	// TODO: support more chunker types
	if opts.GetChunkerArgs().GetChunkerType() != ChunkerType_ChunkerType_RABIN &&
		opts.GetChunkerArgs().GetChunkerType() != ChunkerType_ChunkerType_DEFAULT {
		return errors.Wrap(ErrUnknownChunkerType, opts.GetChunkerArgs().GetChunkerType().String())
	}

	// XXX: this creates a lot of garbage, because multiple writes to the same
	// chunk will create duplicate blocks containing the Chunk data.

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
	lastChunkData, err := lastChunk.FetchData(ctx, lastChunkBcs, false)
	if err != nil {
		return err
	}

	// build new chunk index with last chunk + new data
	chkIdx, totalSize, err := BuildChunkIndex(
		ctx,
		io.MultiReader(bytes.NewReader(lastChunkData), io.LimitReader(rdr, dataLen)),
		chkIdxBcs,
		opts.GetChunkerArgs(),
	)
	if err != nil {
		return err
	}
	b.ChunkIndex, b.TotalSize = chkIdx, totalSize
	bcs.MarkDirty()
	return nil
}

// Truncate changes the length of the blob.
func (b *Blob) Truncate(ctx context.Context, bcs *block.Cursor, blobOpts *BuildBlobOpts, nsize int64) error {
	oldSize := int64(b.GetTotalSize())
	if oldSize == nsize {
		return nil
	}
	if nsize == 0 {
		// clear file
		b.RawData = nil
		b.ChunkIndex = nil
		b.BlobType = 0
		b.TotalSize = uint64(nsize)
		bcs.ClearRef(4)
		bcs.SetBlock(b, true)
		return nil
	}

	hwm := blobOpts.GetRawHighWaterMark()
	if hwm == 0 {
		hwm = DefRawHighWaterMark
	}

	if b.GetBlobType() == BlobType_BlobType_RAW {
		oldSize = int64(len(b.RawData))
		b.TotalSize = uint64(nsize)
		if oldSize < nsize {
			b.RawData = b.RawData[:nsize]
		} else if nsize > int64(hwm) {
			// create a chunk index with the raw data
			// the TotalSize will be used as a limit for reading RawData.
			if err := b.TransformToChunked(ctx, bcs, blobOpts); err != nil {
				return err
			}
		} else {
			// extend buffer if possible
			if cap(b.RawData) >= int(nsize) {
				b.RawData = b.RawData[:nsize]
				// note: optimized to memset by compiler
				for i := int(oldSize); i < len(b.RawData); i++ {
					b.RawData[i] = 0
				}
			} else {
				nraw := make([]byte, nsize)
				copy(nraw, b.RawData)
				b.RawData = nraw
			}
		}
		// done
		return nil
	}

	// assume chunked for the rest of the func
	if b.GetBlobType() != BlobType_BlobType_CHUNKED {
		return errors.Wrap(ErrUnknownBlobType, b.GetBlobType().String())
	}

	// if new size is below high water mark, move to raw blob.
	if hwm >= uint64(nsize) {
		return b.TransformToRaw(ctx, bcs, uint64(nsize))
	}

	// chunk index
	ci := b.GetChunkIndex()
	if ci == nil {
		ci = &ChunkIndex{}
	}

	// truncate chunked blob
	ciBcs := bcs.FollowSubBlock(4)
	ciChunks := ci.GetChunks()
	ciChunksBcs := ciBcs.FollowSubBlock(1)

	// delete any chunks that start outside the new size
	for i := len(ciChunks) - 1; i >= 0; i-- {
		chk := ciChunks[i]
		if chk.GetStart() < uint64(nsize) {
			break
		}
		ciChunks[i] = nil
		ciChunks = ciChunks[:i]
		ciChunksBcs.ClearRef(uint32(i))
	}
	if len(ciChunks) != len(ci.Chunks) {
		if len(ciChunks) == 0 {
			ciChunks = nil
		}
		ci.Chunks = ciChunks
		ciBcs.MarkDirty()
	}

	// shrink the last chunk
	if len(ciChunks) != 0 {
		lastChunkIdx := len(ciChunks) - 1
		lastChunk := ciChunks[lastChunkIdx]
		lastChunkStart, lastChunkSize := lastChunk.GetStart(), lastChunk.GetSize()
		lastChunkEnd := lastChunkStart + lastChunkSize
		if lastChunkEnd > uint64(nsize) {
			nlastChkLen := nsize - int64(lastChunkStart)
			lastChkBcs := ciChunksBcs.FollowSubBlock(uint32(lastChunkIdx))
			// fetch last chunk data
			lastChkData, err := lastChunk.FetchData(ctx, lastChkBcs, false)
			if err != nil {
				return err
			}
			// if necessary, shrink the data field.
			if len(lastChkData) > int(nlastChkLen) {
				lastChkDataBcs := lastChkBcs.FollowRef(1, nil)
				lastChkData = lastChkData[:nlastChkLen]
				lastChkDataBcs.SetBlock(byteslice.NewByteSlice(&lastChkData), true)
			}
			// update the length
			lastChunk.Size = uint64(nlastChkLen)
			lastChkBcs.MarkDirty()
		}
	}

	// update total size
	b.TotalSize = uint64(nsize)
	return nil
}

// TransformToChunked transforms a raw blob to a chunked blob.
func (b *Blob) TransformToChunked(ctx context.Context, bcs *block.Cursor, blobOpts *BuildBlobOpts) error {
	if b.GetBlobType() == 0 || b.GetBlobType() == BlobType_BlobType_CHUNKED {
		return nil
	}
	if b.GetBlobType() != BlobType_BlobType_RAW {
		return errors.Wrap(ErrUnknownBlobType, b.GetBlobType().String())
	}

	// create a chunk index with the raw data with at most totalSize bytes
	totalSize := b.TotalSize
	data := b.RawData
	b.RawData = nil
	return b.WriteChunkIndex(ctx, bcs, blobOpts, io.LimitReader(bytes.NewReader(data), int64(totalSize)))
}

// TransformToRaw transforms a chunked blob to a raw blob.
func (b *Blob) TransformToRaw(ctx context.Context, bcs *block.Cursor, nsize uint64) error {
	if b.GetBlobType() == 0 || b.GetBlobType() == BlobType_BlobType_RAW {
		return nil
	}
	if b.GetBlobType() != BlobType_BlobType_CHUNKED {
		return errors.Wrap(ErrUnknownBlobType, b.GetBlobType().String())
	}

	// chunk index
	ci := b.GetChunkIndex()
	ciBcs := bcs.FollowSubBlock(4)
	ciChunkSet := ci.GetChunkSet(ciBcs)

	nraw := make([]byte, nsize)
	pos := 0
	var rn, chkIdx int
	var err error
	for pos < len(nraw) {
		rn, chkIdx, err = ReadFromChunks(ctx, ciChunkSet, nraw[pos:], pos, chkIdx)
		pos += rn
		if rn == 0 || err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
	}
	b.ChunkIndex = nil
	b.RawData, b.TotalSize = nraw, nsize
	b.BlobType = BlobType_BlobType_RAW
	bcs.ClearRef(4)
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (b *Blob) MarshalBlock() ([]byte, error) {
	return b.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (b *Blob) UnmarshalBlock(data []byte) error {
	return b.UnmarshalVT(data)
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
