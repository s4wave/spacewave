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

// BuildFileWithBytes builds a file with data, building the root blob.
// The new root will be stored at bcs.
func BuildFileWithBytes(
	ctx context.Context,
	btx *block.Transaction,
	bcs *block.Cursor,
	data []byte,
	buildBlobOpts *blob.BuildBlobOpts,
) (*File, *block.Cursor, error) {
	bcs.ClearAllRefs()
	rootBlob, err := blob.BuildBlob(
		ctx,
		uint64(len(data)),
		bytes.NewReader(data),
		bcs,
		buildBlobOpts,
	)
	if err != nil {
		return nil, nil, err
	}
	_, bcs, err = btx.Write()
	if err != nil {
		return nil, nil, err
	}
	bcs.ClearAllRefs()
	fn := NewFileWithBlob(rootBlob)
	bcs.SetBlock(fn)
	return fn, bcs, nil
}
