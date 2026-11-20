package block_store_filesegment

import (
	"context"
	"os"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// LinkFile addresses the file at path with the Space's transform, records
// each chunk's range in idx, and returns the chunked Blob that describes the
// file. Only the Blob goes to the Volume; the chunk bytes stay in the file.
// Every file is chunked, however small, so no file's bytes land in the Blob.
func LinkFile(
	ctx context.Context,
	idx *Index,
	path string,
	args *blob.ChunkerArgs,
	xfrm block.Transformer,
	hashType hash.HashType,
) (*blob.Blob, error) {
	// Record the stat each range is checked against.
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return nil, err
	}
	stat := Location{Path: path, ModTime: fi.ModTime().UnixNano(), Size: fi.Size()}

	// Address the chunks and index each range.
	ci, total, err := blob.AddressChunks(ctx, f, args, xfrm, hashType, func(c *blob.Chunk) error {
		loc := stat
		loc.Offset, loc.Length = int64(c.GetStart()), int64(c.GetSize()) //nolint:gosec // a chunk lies within the file, whose size is an int64.
		idx.Add(c.GetDataRef(), loc)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &blob.Blob{BlobType: blob.BlobType_BlobType_CHUNKED, TotalSize: total, ChunkIndex: ci}, nil
}
