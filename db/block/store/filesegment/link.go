package block_store_filesegment

import (
	"context"
	gohash "hash"
	"io"
	"os"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/net/hash"
)

// LinkAttempts is how many times LinkFile reads a file that changes during
// the read before it gives up with ErrFileChanging.
const LinkAttempts = 3

// ErrFileChanging is returned by LinkFile when the file changed during each
// of LinkAttempts reads. A writer is still at work on it; its next change
// links it again.
var ErrFileChanging = errors.New("file changed during each read")

// errFileChanged reports one read that saw the file change.
var errFileChanged = errors.New("file changed during the read")

// InlineLimit is the largest file LinkFile copies into its Blob instead of
// leaving it in the file. A file this small is one block either way, and
// inlining it saves a peer the round trip for that block, the index its
// entries and a read the stat check against the file.
const InlineLimit = 4 << 10

// LinkFile addresses the file at path, an absolute path, with the Space's
// transform xfrm and hashType, commits each chunk's range to idx in one write,
// and returns the chunked Blob that describes the file. Only the Blob goes to
// the Volume; the chunk bytes stay in the file. A file of at most InlineLimit
// bytes is the exception: its bytes go into a raw Blob, as of link time, and
// idx keeps no range for it. The index write commits before LinkFile returns,
// so a World transaction naming the Blob's refs commits after it.
//
// A file whose stat, or whose path's file, changed by the end of the read is
// read again, so the Blob never mixes two versions of the file; after
// LinkAttempts reads that each saw a change it returns ErrFileChanging and
// writes nothing. A change that keeps the size and the modification time is
// invisible here, and the store's hash check on each read catches it.
//
// sum, if set, is reset and fed the bytes of the version the Blob describes,
// the same read that addresses them.
func LinkFile(
	ctx context.Context,
	idx *Index,
	path string,
	args *blob.ChunkerArgs,
	xfrm block.Transformer,
	hashType hash.HashType,
	sum gohash.Hash,
) (*blob.Blob, error) {
	for range LinkAttempts {
		bl, err := linkFileOnce(ctx, idx, path, args, xfrm, hashType, sum)
		if !errors.Is(err, errFileChanged) {
			return bl, err
		}
	}
	return nil, errors.Wrap(ErrFileChanging, path)
}

// linkFileOnce reads and links the file at path once, feeding sum, if set,
// its bytes, and returns errFileChanged when the file changed during the
// read.
func linkFileOnce(
	ctx context.Context,
	idx *Index,
	path string,
	args *blob.ChunkerArgs,
	xfrm block.Transformer,
	hashType hash.HashType,
	sum gohash.Hash,
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

	// Inline a small file, dropping any ranges an earlier, larger version left.
	// A file that grew since the stat is chunked from its start.
	if size <= InlineLimit {
		data, err := io.ReadAll(io.LimitReader(f, InlineLimit+1))
		if err != nil {
			return nil, err
		}
		if len(data) <= InlineLimit {
			if err := checkUnchanged(f, path, fi); err != nil {
				return nil, err
			}
			if err := idx.DropPath(ctx, path); err != nil {
				return nil, err
			}
			if sum != nil {
				sum.Reset()
				_, _ = sum.Write(data)
			}
			return blob.NewRawBlob(data), nil
		}
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return nil, err
		}
	}

	// Address the chunks, keeping each range. A file changing under the read
	// stops it at the next chunk, so an attempt on a file being written costs
	// the bytes read until its next write, not the whole file.
	var rdr io.Reader = f
	if sum != nil {
		sum.Reset()
		rdr = io.TeeReader(f, sum)
	}
	var refs []*block.BlockRef
	var segs []*Segment
	ci, total, err := blob.AddressChunks(ctx, rdr, args, xfrm, hashType, func(c *blob.Chunk, blockSize int) error {
		// Stop at a change.
		now, err := f.Stat()
		if err != nil {
			return err
		}
		if !sameStat(now, fi) {
			return errFileChanged
		}

		// Keep the chunk's range.
		refs = append(refs, c.GetDataRef())
		segs = append(segs, &Segment{
			Path:      path,
			Offset:    int64(c.GetStart()), //nolint:gosec // a chunk lies within the file, whose size is an int64.
			Length:    int64(c.GetSize()),  //nolint:gosec // as above.
			ModTime:   modTime,
			Size:      size,
			BlockSize: int64(blockSize),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Replace the path's ranges in one write, if the read saw one version.
	if err := checkUnchanged(f, path, fi); err != nil {
		return nil, err
	}
	if err := idx.SetPath(ctx, path, refs, segs); err != nil {
		return nil, err
	}
	return &blob.Blob{BlobType: blob.BlobType_BlobType_CHUNKED, TotalSize: total, ChunkIndex: ci}, nil
}

// checkUnchanged returns errFileChanged unless the open file f and the file
// now at path are both the file of stat fi, with its size and modification
// time.
func checkUnchanged(f *os.File, path string, fi os.FileInfo) error {
	// Compare the open file's stat.
	now, err := f.Stat()
	if err != nil {
		return err
	}
	if !sameStat(now, fi) {
		return errFileChanged
	}

	// Compare the file at the path, which a rename may have replaced.
	at, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return errFileChanged
	}
	if err != nil {
		return err
	}
	if !sameFile(at, fi) || !sameStat(at, fi) {
		return errFileChanged
	}
	return nil
}

// sameStat reports whether a and b have the same size and modification time.
func sameStat(a, b os.FileInfo) bool {
	return a.Size() == b.Size() && a.ModTime().Equal(b.ModTime())
}
