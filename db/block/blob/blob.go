package blob

import (
	"bytes"
	"context"
	"io"
	"math"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
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
	// Decode and validate the root before selecting its storage representation.
	root, err := UnmarshalBlob(ctx, bcs)
	if err != nil {
		return err
	}
	if err := root.GetBlobType().Validate(); err != nil {
		return err
	}

	// Skip storage reads for an empty blob.
	if root.GetTotalSize() == 0 {
		return nil
	}

	// Copy raw bytes directly or stream chunk data through a blob reader.
	switch root.GetBlobType() {
	case BlobType_BlobType_RAW:
		if root.GetTotalSize() > math.MaxInt {
			return errors.New("total size exceeds maximum")
		}

		if len(root.GetRawData()) != int(root.GetTotalSize()) { //nolint:gosec
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
	// Fetch the complete blob into a temporary buffer before returning bytes.
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
	// Validate empty-state fields before checking representation-specific data.
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

	// Validate raw payload size and reject raw bytes on chunked blobs.
	if blobType == BlobType_BlobType_RAW {
		if b.GetTotalSize() > math.MaxInt {
			return errors.New("total size exceeds maximum")
		}

		if len(b.GetRawData()) != int(b.GetTotalSize()) { //nolint:gosec
			return ErrRawBlobSizeMismatch
		}
	} else if len(b.GetRawData()) != 0 {
		return errors.New("raw_data field must be empty for non-raw blob")
	}

	// Require an empty chunk index for non-chunked values.
	if blobType != BlobType_BlobType_CHUNKED {
		if len(b.GetChunkIndex().GetChunks()) != 0 {
			return errors.New("expected empty chunks field for non-chunked blob type")
		}
		return nil
	}
	return b.GetChunkIndex().Validate()
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

	// Fetch the root block so its encoded bytes contribute to storage size.
	rootData, _, err := bcs.Fetch(ctx)
	if err != nil {
		return 0, 0, err
	}
	storageSize += uint64(len(rootData))

	// Return root-only size for raw blobs and deduplicate chunk storage sizes.
	if b.GetBlobType() != BlobType_BlobType_CHUNKED {
		return storageSize, storageSize, nil
	}

	// Add chunk payload sizes without fetching chunk bodies.
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

		// Count each content-addressed chunk block once for physical storage.
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
	// Validate root metadata before deciding whether chunk data can be read.
	if err := b.GetBlobType().Validate(); err != nil {
		return err
	}

	blobType := b.GetBlobType()
	if b.GetTotalSize() > math.MaxInt64 {
		return errors.New("total size exceeds maximum")
	}

	totalSize := int64(b.GetTotalSize()) //nolint:gosec
	if totalSize == 0 {
		if blobType != BlobType_BlobType_RAW {
			return errors.New("empty blobs must be of raw type")
		}
		return nil
	}

	// Validate raw payloads directly before requiring a cursor for chunks.
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

	// Without a cursor, validate the root fields but skip chunk reads.
	if bcs == nil {
		return nil
	}

	// Stream every chunk and enforce the declared total size.
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
	// Build the chunk index and update the blob's chunked metadata.
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
	// Resolve append mode from the high-water mark and existing blob type.
	hwm := opts.GetRawHighWaterMark()
	if hwm == 0 {
		hwm = DefRawHighWaterMark
	}

	// Extend raw data in place when the next size stays below the high-water mark.
	oldLen := b.GetTotalSize()

	nextLen := oldLen + uint64(dataLen) //nolint:gosec
	if b.GetBlobType() == BlobType_BlobType_RAW {
		if nextLen <= hwm {
			// Extend the raw buffer while it remains below the high-water mark.
			ndata := make([]byte, nextLen)
			_, err := io.ReadAtLeast(rdr, ndata[oldLen:], int(dataLen))
			if err != nil {
				return err
			}
			copy(ndata[:oldLen], b.GetRawData())
			b.RawData = ndata
			b.TotalSize = nextLen
		} else {
			// Rechunk the existing raw bytes together with the appended reader.
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

	// Reject unsupported types before preparing the existing chunk index.
	if b.GetBlobType() != BlobType_BlobType_CHUNKED {
		return errors.Errorf("cannot extend blob type: %s", b.GetBlobType().String())
	}
	if b.ChunkIndex == nil {
		b.ChunkIndex = &ChunkIndex{}
	}

	// XXX: this creates a lot of garbage, because multiple writes to the same
	// chunk will create duplicate blocks containing the Chunk data.

	// append to existing chunked blob
	chkIdxBcs := bcs.FollowSubBlock(4)
	chunks := b.GetChunkIndex().GetChunks()
	if len(chunks) == 0 {
		return b.WriteChunkIndex(ctx, bcs, opts, io.LimitReader(rdr, dataLen))
	}

	// Remove the final chunk so its data can be combined with appended input.
	chunksSet := NewChunkSet(&chunks, chkIdxBcs.FollowSubBlock(1))

	// Remove the last chunk so its bytes can be combined with new input.
	lastChunkIdx := len(chunks) - 1
	lastChunk := chunks[lastChunkIdx]
	_, lastChunkBcs := chunksSet.Get(lastChunkIdx)

	chunksSet.GetCursor().ClearRef(uint32(lastChunkIdx)) //nolint:gosec
	chunks = chunks[:lastChunkIdx]
	b.ChunkIndex.Chunks = chunks

	// Fetch the removed tail and rebuild the chunk index with new input.
	// Fetch the removed chunk before rebuilding the tail index.
	lastChunkData, err := lastChunk.FetchData(ctx, lastChunkBcs, false)
	if err != nil {
		return err
	}

	// Rebuild the chunk index from the old tail and appended data.
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
	// Validate size bounds before selecting clear, raw, or chunked truncation.
	if b.GetTotalSize() > math.MaxInt64 {
		return errors.New("total size exceeds maximum")
	}

	// Clear all references and metadata when truncating to an empty blob.
	oldSize := int64(b.GetTotalSize()) //nolint:gosec
	if oldSize == nsize {
		return nil
	}
	if nsize == 0 {
		// Clear all root data and references for an empty blob.
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

	// Resize raw storage in place or promote it when the new size is large.
	// Resize raw data in place or promote it to a chunk index.
	if b.GetBlobType() == BlobType_BlobType_RAW {
		oldSize = int64(len(b.RawData))

		b.TotalSize = uint64(nsize) //nolint:gosec
		if oldSize < nsize {
			b.RawData = b.RawData[:nsize]
		} else if nsize > int64(hwm) { //nolint:gosec

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

	// Convert small chunked blobs to raw storage when the high-water mark permits.
	// if new size is below high water mark, move to raw blob.
	if hwm >= uint64(nsize) { //nolint:gosec

		return b.TransformToRaw(ctx, bcs, uint64(nsize)) //nolint:gosec
	}

	// chunk index
	ci := b.GetChunkIndex()
	if ci == nil {
		ci = &ChunkIndex{}
	}

	// Remove chunks beyond the new end and shorten the retained tail.
	// Remove chunks outside the new end and shorten the final retained chunk.
	ciBcs := bcs.FollowSubBlock(4)
	ciChunks := ci.GetChunks()
	ciChunksBcs := ciBcs.FollowSubBlock(1)

	// delete any chunks that start outside the new size
	for i, v := range slices.Backward(ciChunks) {
		chk := v

		if chk.GetStart() < uint64(nsize) { //nolint:gosec
			break
		}
		ciChunks = ciChunks[:i]

		ciChunksBcs.ClearRef(uint32(i)) //nolint:gosec
	}
	if len(ciChunks) != len(ci.Chunks) {
		if len(ciChunks) == 0 {
			ciChunks = nil
		}
		ci.Chunks = ciChunks
		ciBcs.MarkDirty()
	}

	// Fetch and rewrite the final chunk when it extends beyond the new size.
	// shrink the last chunk
	if len(ciChunks) != 0 {
		lastChunkIdx := len(ciChunks) - 1
		lastChunk := ciChunks[lastChunkIdx]
		lastChunkStart, lastChunkSize := lastChunk.GetStart(), lastChunk.GetSize()
		lastChunkEnd := lastChunkStart + lastChunkSize

		if lastChunkEnd > uint64(nsize) { //nolint:gosec
			if lastChunkStart > math.MaxInt64 {
				return errors.New("chunk start exceeds maximum")
			}
			nlastChkLen := nsize - int64(lastChunkStart)

			lastChkBcs := ciChunksBcs.FollowSubBlock(uint32(lastChunkIdx)) //nolint:gosec

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
			lastChunk.Size = uint64(nlastChkLen) //nolint:gosec
			lastChkBcs.MarkDirty()
		}
	}

	// Record the final truncated size after chunk references are reconciled.
	// update total size
	b.TotalSize = uint64(nsize) //nolint:gosec
	return nil
}

// TransformToChunked transforms a raw blob to a chunked blob.
func (b *Blob) TransformToChunked(ctx context.Context, bcs *block.Cursor, blobOpts *BuildBlobOpts) error {
	// Convert raw data into a chunk index while preserving the declared size.
	if b.GetBlobType() == 0 || b.GetBlobType() == BlobType_BlobType_CHUNKED {
		return nil
	}
	if b.GetBlobType() != BlobType_BlobType_RAW {
		return errors.Wrap(ErrUnknownBlobType, b.GetBlobType().String())
	}

	// create a chunk index with the raw data with at most totalSize bytes
	totalSize := b.TotalSize
	if totalSize > math.MaxInt64 {
		return errors.New("total size exceeds maximum")
	}
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

	// Read chunk data into contiguous raw storage and clear the chunk index.
	// chunk index
	ci := b.GetChunkIndex()
	ciBcs := bcs.FollowSubBlock(4)
	ciChunkSet := ci.GetChunkSet(ciBcs)

	// Read chunk data into a contiguous raw buffer up to the requested size.
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
	_ block.Block              = (*Blob)(nil)
	_ block.BlockWithSubBlocks = (*Blob)(nil)
)
