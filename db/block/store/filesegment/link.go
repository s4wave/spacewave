package block_store_filesegment

import (
	"context"
	"os"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// LinkFile addresses the file at path, an absolute path, with the Space's
// transform xfrm and hashType, commits each chunk's range to idx in one write,
// and returns the chunked Blob that describes the file. Only the Blob goes to
// the Volume; the chunk bytes stay in the file. Every file is chunked, however
// small, so no file's bytes land in the Blob. The index write commits before
// LinkFile returns, so a World transaction naming the Blob's refs commits after
// it.
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
	modTime, size := fi.ModTime().UnixNano(), fi.Size()

	// Address the chunks, keeping each range.
	var refs []*block.BlockRef
	var segs []*Segment
	ci, total, err := blob.AddressChunks(ctx, f, args, xfrm, hashType, func(c *blob.Chunk) error {
		refs = append(refs, c.GetDataRef())
		segs = append(segs, &Segment{
			Path:    path,
			Offset:  int64(c.GetStart()), //nolint:gosec // a chunk lies within the file, whose size is an int64.
			Length:  int64(c.GetSize()),  //nolint:gosec // as above.
			ModTime: modTime,
			Size:    size,
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Replace the path's ranges in one write.
	if err := idx.SetPath(ctx, path, refs, segs); err != nil {
		return nil, err
	}
	return &blob.Blob{BlobType: blob.BlobType_BlobType_CHUNKED, TotalSize: total, ChunkIndex: ci}, nil
}
