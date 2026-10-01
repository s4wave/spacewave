package blob

import (
	"bytes"
	"context"
	"io"
	"math"
	"slices"
	"sort"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
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

// AppendData appends dataLen bytes from rdr to the end of the blob.
//
// A small raw blob grows in place. A chunked blob keeps the bytes after its
// last chunk boundary as tail chunks: an append that keeps the tail within the
// maximum chunk size stores only its own bytes as one more tail chunk, and a
// larger one chunks the tail together with the new bytes.
func (b *Blob) AppendData(
	ctx context.Context,
	dataLen int64,
	rdr io.Reader,
	bcs *block.Cursor,
	opts *BuildBlobOpts,
) error {
	switch b.GetBlobType() {
	case BlobType_BlobType_RAW:
		return b.appendRaw(ctx, dataLen, rdr, bcs, opts)
	case BlobType_BlobType_CHUNKED:
		return b.appendChunked(ctx, dataLen, rdr, bcs, opts)
	default:
		return errors.Errorf("cannot extend blob type: %s", b.GetBlobType().String())
	}
}

// appendRaw appends to a raw blob. Each append rewrites the raw data, so only
// the first write or a blob within DefRawAppendLimit stays raw.
func (b *Blob) appendRaw(
	ctx context.Context,
	dataLen int64,
	rdr io.Reader,
	bcs *block.Cursor,
	opts *BuildBlobOpts,
) error {
	// Resolve the high-water mark.
	hwm := opts.GetRawHighWaterMark()
	if hwm == 0 {
		hwm = DefRawHighWaterMark
	}

	// Extend the raw data in place while the blob stays small.
	oldLen := b.GetTotalSize()
	nextLen := oldLen + uint64(dataLen) //nolint:gosec
	if nextLen <= hwm && (oldLen == 0 || nextLen <= DefRawAppendLimit) {
		ndata := make([]byte, nextLen)
		if _, err := io.ReadFull(rdr, ndata[oldLen:]); err != nil {
			return err
		}
		copy(ndata, b.GetRawData())
		b.RawData = ndata
		b.TotalSize = nextLen
		bcs.SetBlock(b, true)
		return nil
	}

	// Chunk the raw data together with the appended bytes.
	mrdr := io.MultiReader(bytes.NewReader(b.GetRawData()), io.LimitReader(rdr, dataLen))
	if err := b.WriteChunkIndex(ctx, bcs, opts, mrdr); err != nil {
		return err
	}
	b.RawData = nil
	bcs.SetBlock(b, true)
	return nil
}

// appendChunked appends to a chunked blob through its tail.
func (b *Blob) appendChunked(
	ctx context.Context,
	dataLen int64,
	rdr io.Reader,
	bcs *block.Cursor,
	opts *BuildBlobOpts,
) error {
	// Find the end of the chunked data and of the append.
	if b.ChunkIndex == nil {
		b.ChunkIndex = &ChunkIndex{}
	}
	ci := b.ChunkIndex
	ciBcs := bcs.FollowSubBlock(4)
	chunks := ci.GetChunks()
	end := ci.GetEnd()
	nextEnd := end + uint64(dataLen) //nolint:gosec

	// Find the first tail chunk.
	tailStart := min(ci.GetTailStart(), end)
	tailIdx := sort.Search(len(chunks), func(i int) bool {
		return chunks[i].GetStart() >= tailStart
	})

	// Store an append that fits in the tail as one more tail chunk.
	args := ci.GetChunkerArgs().CloneVT()
	if args == nil {
		args = &ChunkerArgs{}
	}
	args.ApplyArgs(opts.GetChunkerArgs())
	if nextEnd-tailStart <= args.GetMaxChunkSize() {
		data := make([]byte, dataLen)
		if _, err := io.ReadFull(rdr, data); err != nil {
			return err
		}
		pieces := newChunkAppender(ctx, ci, ci.GetChunkSet(ciBcs))
		if err := pieces.append(pieces.next(), uint64(dataLen), end, data); err != nil { //nolint:gosec
			return err
		}
		if err := pieces.flush(); err != nil {
			return err
		}
		ci.TailStart = tailStart
		b.TotalSize = nextEnd
		ciBcs.SetBlock(ci, true)
		bcs.MarkDirty()
		return nil
	}

	// Read the tail chunks, then remove them from the index.
	tail := slices.Clone(chunks[tailIdx:])
	chunkSet := ci.GetChunkSet(ciBcs)
	rdrs := make([]io.Reader, 0, len(tail)+1)
	for i, chk := range tail {
		_, chkBcs := chunkSet.Get(tailIdx + i)
		data, err := chk.FetchData(ctx, chkBcs, false)
		if err != nil {
			return err
		}
		rdrs = append(rdrs, bytes.NewReader(data))
	}
	rdrs = append(rdrs, io.LimitReader(rdr, dataLen))
	for i := len(chunks) - 1; i >= tailIdx; i-- {
		chunkSet.GetCursor().ClearRef(uint32(i)) //nolint:gosec
	}
	ci.Chunks = chunks[:tailIdx]

	// Chunk the tail and the appended bytes, holding back the bytes after the
	// last boundary.
	nci, _, held, err := buildChunkIndex(ctx, io.MultiReader(rdrs...), ciBcs, opts.GetChunkerArgs(), true)
	if err != nil {
		return err
	}

	// Keep the held bytes as the new tail.
	if held != nil {
		pieces := newChunkAppender(ctx, nci, nci.GetChunkSet(ciBcs))
		if err := pieces.appendTail(tail, held); err != nil {
			return err
		}
	}
	b.ChunkIndex, b.TotalSize = nci, nextEnd
	ciBcs.SetBlock(nci, true)
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
	if nsize < 0 {
		return errors.New("negative blob size")
	}
	oldSize := int64(b.GetTotalSize()) //nolint:gosec
	if oldSize == nsize {
		return nil
	}
	if nsize == 0 {
		b.RawData = nil
		b.ChunkIndex = nil
		b.BlobType = 0
		b.TotalSize = 0
		bcs.ClearRef(4)
		bcs.SetBlock(b, true)
		return nil
	}

	// Resolve the high-water mark between raw and chunked storage.
	hwm := blobOpts.GetRawHighWaterMark()
	if hwm == 0 {
		hwm = DefRawHighWaterMark
	}

	// Resize raw data, zero filling growth, and chunk it past the high-water
	// mark.
	size := uint64(nsize) //nolint:gosec
	if b.GetBlobType() == BlobType_BlobType_RAW {
		if nsize < int64(len(b.RawData)) {
			b.RawData = b.RawData[:nsize]
		} else {
			nraw := make([]byte, nsize)
			copy(nraw, b.RawData)
			b.RawData = nraw
		}
		b.TotalSize = size
		bcs.SetBlock(b, true)
		if size > hwm {
			return b.TransformToChunked(ctx, bcs, blobOpts)
		}
		return nil
	}

	// Reject unknown types, and move a chunked blob that fits under the
	// high-water mark to raw storage.
	if b.GetBlobType() != BlobType_BlobType_CHUNKED {
		return errors.Wrap(ErrUnknownBlobType, b.GetBlobType().String())
	}
	if hwm >= size {
		return b.TransformToRaw(ctx, bcs, size)
	}

	// Growth past the chunks reads as zeros, so only shrinking changes chunks.
	b.TotalSize = size
	bcs.MarkDirty()
	if nsize > oldSize {
		return nil
	}

	// Count the chunks that start before the new end.
	if b.ChunkIndex == nil {
		b.ChunkIndex = &ChunkIndex{}
	}
	ci := b.ChunkIndex
	ciBcs := bcs.FollowSubBlock(4)
	chunkSet := ci.GetChunkSet(ciBcs)
	chunks := ci.GetChunks()
	keep := sort.Search(len(chunks), func(i int) bool {
		return chunks[i].GetStart() >= size
	})

	// Remove the chunks past the new end.
	for i := len(chunks) - 1; i >= keep; i-- {
		chunkSet.GetCursor().ClearRef(uint32(i)) //nolint:gosec
	}
	ci.Chunks = chunks[:keep]
	ciBcs.MarkDirty()
	if keep == 0 {
		ci.TailStart = 0
		return nil
	}

	// Start the tail no later than the last kept chunk, which may now end
	// before its boundary.
	last := chunks[keep-1]
	lastStart, lastEnd := last.GetStart(), last.GetStart()+last.GetSize()
	ci.TailStart = min(ci.GetTailStart(), lastStart)
	if lastEnd <= size {
		return nil
	}

	// Read a last chunk that extends past the new end, and remove it.
	_, lastBcs := chunkSet.Get(keep - 1)
	data, err := last.FetchData(ctx, lastBcs, false)
	if err != nil {
		return err
	}
	chunkSet.GetCursor().ClearRef(uint32(keep - 1)) //nolint:gosec
	ci.Chunks = chunks[:keep-1]

	// Store its prefix in its place.
	nlen := size - lastStart
	pieces := newChunkAppender(ctx, ci, chunkSet)
	if err := pieces.append(keep-1, nlen, lastStart, data[:nlen]); err != nil {
		return err
	}
	return pieces.flush()
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
