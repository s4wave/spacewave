package file

import (
	"bytes"
	"context"
	"io"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
)

// NewFileWithBlob builds a file with a single root blob.
func NewFileWithBlob(rootBlob *blob.Blob) *File {
	return &File{
		TotalSize: rootBlob.GetTotalSize(),
		RootBlob:  rootBlob,
	}
}

// BuildFileWithBytes builds a file with data, building the root blob.
// The new root will be stored at bcs.
func BuildFileWithBytes(
	ctx context.Context,
	bcs *block.Cursor,
	data []byte,
	buildBlobOpts *blob.BuildBlobOpts,
) (*File, error) {
	// Initialize the file root and clear any previous cursor references.
	totalSize := uint64(len(data))
	fn := &File{TotalSize: totalSize}
	bcs.ClearAllRefs()
	bcs.SetBlock(fn, true)

	// Build and attach the root blob beneath the initialized file.
	rootBlobCs := bcs.FollowSubBlock(2)
	rootBlob, err := blob.BuildBlob(
		ctx,
		int64(len(data)),
		bytes.NewReader(data),
		rootBlobCs,
		buildBlobOpts,
	)
	fn.RootBlob = rootBlob
	return fn, err
}

// BuildFileWithReader builds a file with a reader, building the root blob.
// The new root will be stored at bcs.
func BuildFileWithReader(
	ctx context.Context,
	bcs *block.Cursor,
	rdr io.Reader,
	buildBlobOpts *blob.BuildBlobOpts,
) (*File, error) {
	// Initialize an empty file root before streaming reader data.
	fn := &File{}
	bcs.ClearAllRefs()
	bcs.SetBlock(fn, true)

	// Build and attach the streamed root blob, then record its size.
	rootBlobCs := bcs.FollowSubBlock(2)
	rootBlob, err := blob.BuildBlobWithReader(
		ctx,
		rdr,
		rootBlobCs,
		buildBlobOpts,
	)

	fn.RootBlob = rootBlob
	fn.TotalSize = rootBlob.GetTotalSize()

	return fn, err
}
