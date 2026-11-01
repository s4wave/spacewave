package packfile

import (
	"context"
	"io"
	"math"
	"strconv"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
)

// ReadIndex reads the key index of a packfile of size bytes from ra, without
// its block values. The entries are in key order.
func ReadIndex(ra io.ReaderAt, size uint64) ([]*kvfile.IndexEntry, error) {
	// Read the index at the tail of the packfile.
	_, tail, err := kvfile.ReadIndexTail(ra, size)
	if err != nil {
		return nil, errors.Wrap(err, "read packfile index")
	}

	// Collect its entries.
	var index []*kvfile.IndexEntry
	err = ScanIndexTail(tail, size, func(ie *kvfile.IndexEntry) error {
		index = append(index, ie.CloneVT())
		return nil
	})
	return index, err
}

// IndexTailFirstRead is the length of the first read from the end of a
// packfile for its index tail. It covers the whole tail of a typical pack, so
// one read usually suffices.
const IndexTailFirstRead = 812 << 10

// maxIndexEntrySize is the largest index entry go-kvfile writes. A read that
// reaches this far below the first entry's length varint covers the entry.
const maxIndexEntrySize = 2048

// ReadIndexTail returns the index tail of a packfile of size bytes, reading
// backwards from its end with read, which returns exactly length bytes at off.
// The first read takes the last IndexTailFirstRead bytes. When the tail starts
// below what was read, each further read takes only the missing prefix, sized
// by the kvfile trailer, so a tail costs at most three reads and at most
// maxIndexEntrySize bytes beyond the tail and the first read.
func ReadIndexTail(ctx context.Context, size uint64, read func(ctx context.Context, off int64, length int) ([]byte, error)) ([]byte, error) {
	// Offsets into the packfile are int64.
	if size > math.MaxInt64 {
		return nil, errors.Errorf("packfile size %d overflows int64", size)
	}

	// Read the end of the packfile, then each missing prefix the trailer asks for.
	base := size - min(size, IndexTailFirstRead)
	suffix, err := readIndexRange(ctx, read, base, size)
	if err != nil {
		return nil, err
	}
	for {
		start, err := kvfile.IndexTailOffset(&suffixReaderAt{base: base, data: suffix}, size)
		var miss *suffixMiss
		switch {
		case errors.As(err, &miss):
			start = miss.off - min(miss.off, maxIndexEntrySize)
		case err != nil:
			return nil, errors.Wrap(err, "locate packfile index")
		case start >= base:
			return suffix[start-base:], nil
		}

		// Prepend the missing prefix [start, base).
		prefix, err := readIndexRange(ctx, read, start, base)
		if err != nil {
			return nil, err
		}
		suffix = append(prefix, suffix...)
		base = start
	}
}

// readIndexRange reads [start, end) of a packfile with read.
func readIndexRange(ctx context.Context, read func(ctx context.Context, off int64, length int) ([]byte, error), start, end uint64) ([]byte, error) {
	// An empty range needs no read.
	if end == start {
		return nil, nil
	}
	data, err := read(ctx, int64(start), int(end-start)) //nolint:gosec // end is at most a size checked against MaxInt64.
	if err != nil {
		return nil, errors.Wrap(err, "read packfile index")
	}
	if uint64(len(data)) != end-start {
		return nil, errors.Errorf("read packfile index at %d: got %d bytes, want %d", start, len(data), end-start)
	}
	return data, nil
}

// suffixReaderAt reads the bytes of a packfile from base to its end, and
// reports a read that reaches below base as a suffixMiss.
type suffixReaderAt struct {
	base uint64
	data []byte
}

// ReadAt reads len(p) bytes at off.
func (r *suffixReaderAt) ReadAt(p []byte, off int64) (int, error) {
	// Report a read below base as a miss at its offset.
	if off < 0 {
		return 0, errors.Errorf("negative offset %d", off)
	}
	if uint64(off) < r.base {
		return 0, &suffixMiss{off: uint64(off)}
	}

	// Copy what the suffix holds at off.
	rel := uint64(off) - r.base
	if rel >= uint64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[rel:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// suffixMiss reports a read at off, below the bytes read so far.
type suffixMiss struct {
	off uint64
}

// Error describes the missing offset.
func (m *suffixMiss) Error() string {
	return "packfile offset " + strconv.FormatUint(m.off, 10) + " is below the bytes read"
}

// ScanIndexTail calls fn with each index entry of the packfile of size bytes
// whose index tail is tail, in key order, without collecting them. fn must copy
// what it keeps of the entry.
func ScanIndexTail(tail []byte, size uint64, fn func(*kvfile.IndexEntry) error) error {
	// Open a reader over the tail alone, then scan its entries.
	rd, err := kvfile.BuildReaderWithIndexTail(tail, size)
	if err != nil {
		return errors.Wrap(err, "open packfile index")
	}
	err = rd.ScanPrefixEntries(nil, func(ie *kvfile.IndexEntry, _ int) error {
		return fn(ie)
	})
	if err != nil {
		return errors.Wrap(err, "scan packfile index")
	}
	return nil
}

// _ is a type assertion
var (
	_ io.ReaderAt = (*suffixReaderAt)(nil)
	_ error       = (*suffixMiss)(nil)
)
