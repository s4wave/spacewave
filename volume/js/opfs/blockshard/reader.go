//go:build js

package blockshard

import (
	"encoding/binary"
	"syscall/js"

	"github.com/aperturerobotics/hydra/opfs"
	"github.com/aperturerobotics/hydra/volume/js/opfs/segment"
	"github.com/pkg/errors"
)

// SegmentReader reads entries from a sealed SSTable segment file
// using async getFile().slice() reads. No WebLock or sync handle needed.
type SegmentReader struct {
	file   *opfs.AsyncFile
	size   int64
	header *segment.Header
	minKey []byte
	maxKey []byte
	index  []segment.IndexEntry
	bloom  *segment.BloomFilter
}

// OpenSegment opens a sealed segment file for reading via async file access.
func OpenSegment(dir js.Value, filename string) (*SegmentReader, error) {
	f, err := opfs.OpenAsyncFile(dir, filename)
	if err != nil {
		return nil, errors.Wrap(err, "open segment file")
	}

	size, err := f.Size()
	if err != nil {
		return nil, errors.Wrap(err, "get segment size")
	}

	if size < segment.HeaderSize+4 {
		return nil, errors.New("segment file too small")
	}

	// Read and validate the full file for CRC32 verification.
	// For production, we could validate lazily, but correctness first.
	contentSize := size - 4
	content := make([]byte, contentSize)
	if _, err := f.ReadAt(content, 0); err != nil {
		return nil, errors.Wrap(err, "read segment content")
	}

	var footerBuf [4]byte
	if _, err := f.ReadAt(footerBuf[:], contentSize); err != nil {
		return nil, errors.Wrap(err, "read segment footer")
	}

	// Verify CRC32 using the segment package's import.
	expected := binary.BigEndian.Uint32(footerBuf[:])
	rd, err := segment.NewReader(newByteReaderAt(content, footerBuf[:]), size)
	if err != nil {
		return nil, errors.Wrap(err, "parse segment")
	}
	_ = expected // CRC32 validated inside NewReader

	sr := &SegmentReader{
		file:   f,
		size:   size,
		header: rd.Header(),
		minKey: rd.MinKey(),
		maxKey: rd.MaxKey(),
		index:  rd.Index(),
		bloom:  rd.Bloom(),
	}
	return sr, nil
}

// Get looks up a key in this segment. Returns value, found, error.
// Uses bloom filter for fast rejection, sparse index for window narrowing,
// then linear scan within the data window. All reads are async (no WebLock).
func (sr *SegmentReader) Get(key []byte) ([]byte, bool, error) {
	// Range check.
	if string(key) < string(sr.minKey) || string(key) > string(sr.maxKey) {
		return nil, false, nil
	}

	// Bloom filter fast rejection.
	if sr.bloom != nil && !sr.bloom.MayContain(key) {
		return nil, false, nil
	}

	// Narrow scan window via sparse index.
	dataSize := sr.header.DataSize
	start, limit := segment.SearchIndex(sr.index, key, dataSize)
	windowSize := limit - start

	// Read the data window via async slice.
	window := make([]byte, windowSize)
	if _, err := sr.file.ReadAt(window, int64(sr.header.DataOffset)+int64(start)); err != nil {
		return nil, false, errors.Wrap(err, "read data window")
	}

	// Linear scan within the window.
	off := 0
	for off < len(window) {
		if off+2 > len(window) {
			break
		}
		keyLen := int(binary.BigEndian.Uint16(window[off : off+2]))
		off += 2
		if off+keyLen > len(window) {
			break
		}
		entryKey := window[off : off+keyLen]
		off += keyLen
		if off+4 > len(window) {
			break
		}
		valLen := binary.BigEndian.Uint32(window[off : off+4])
		off += 4

		k := string(entryKey)
		if k == string(key) {
			if valLen == segment.TombstoneLen {
				return nil, false, nil
			}
			if off+int(valLen) > len(window) {
				return nil, false, errors.New("truncated value")
			}
			val := make([]byte, valLen)
			copy(val, window[off:off+int(valLen)])
			return val, true, nil
		}
		if k > string(key) {
			return nil, false, nil
		}
		if valLen != segment.TombstoneLen {
			off += int(valLen)
		}
	}
	return nil, false, nil
}

// MinKey returns the smallest key in the segment.
func (sr *SegmentReader) MinKey() []byte { return sr.minKey }

// MaxKey returns the largest key in the segment.
func (sr *SegmentReader) MaxKey() []byte { return sr.maxKey }

// EntryCount returns the number of entries.
func (sr *SegmentReader) EntryCount() uint32 { return sr.header.EntryCount }

// byteReaderAt wraps content + footer into an io.ReaderAt for segment.NewReader.
type byteReaderAt struct {
	data []byte
}

func newByteReaderAt(content, footer []byte) *byteReaderAt {
	combined := make([]byte, len(content)+len(footer))
	copy(combined, content)
	copy(combined[len(content):], footer)
	return &byteReaderAt{data: combined}
}

func (b *byteReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b.data)) {
		return 0, errors.New("offset past end")
	}
	n := copy(p, b.data[off:])
	return n, nil
}
