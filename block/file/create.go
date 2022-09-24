package file

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/block/blob"
)

// NewFileWithBlob builds a file with a single root blob.
func NewFileWithBlob(rootBlob *blob.Blob) *File {
	return &File{
		TotalSize: rootBlob.GetTotalSize(),
		RootBlob:  rootBlob,
	}
}

// BuildFile builds a file from a

// BuildFileWithBytes builds a file with data, building the root blob.
// The new root will be stored at bcs.
func BuildFileWithBytes(
	ctx context.Context,
	bcs *block.Cursor,
	data []byte,
	buildBlobOpts *blob.BuildBlobOpts,
) (*File, error) {
	totalSize := uint64(len(data))
	fn := &File{TotalSize: totalSize}
	bcs.ClearAllRefs()
	bcs.SetBlock(fn, true)

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
