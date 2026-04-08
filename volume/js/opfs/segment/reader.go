package segment

import (
	"encoding/binary"
	"hash/crc32"
	"io"

	"github.com/pkg/errors"
)

// Reader reads entries from an SSTable via an io.ReaderAt.
type Reader struct {
	r      io.ReaderAt
	size   int64
	header *Header
	minKey []byte
	maxKey []byte
	index  []IndexEntry
	bloom  *BloomFilter
}

// NewReader opens an SSTable for reading. size is the total file length.
func NewReader(r io.ReaderAt, size int64) (*Reader, error) {
	if size < HeaderSize+4 {
		return nil, errors.New("file too small for SSTable")
	}

	// Read and verify CRC32 footer.
	contentSize := size - 4
	content := make([]byte, contentSize)
	if _, err := r.ReadAt(content, 0); err != nil {
		return nil, errors.Wrap(err, "read content")
	}

	var footerBuf [4]byte
	if _, err := r.ReadAt(footerBuf[:], contentSize); err != nil {
		return nil, errors.Wrap(err, "read footer")
	}
	expected := binary.BigEndian.Uint32(footerBuf[:])
	actual := crc32.ChecksumIEEE(content)
	if expected != actual {
		return nil, errors.Errorf("CRC32 mismatch: expected %08x, got %08x", expected, actual)
	}

	// Parse header.
	hdr, err := DecodeHeader(content[:HeaderSize])
	if err != nil {
		return nil, errors.Wrap(err, "decode header")
	}

	// Read min and max keys.
	off := HeaderSize
	if off+2 > len(content) {
		return nil, errors.New("truncated min key length")
	}
	minKeyLen := binary.BigEndian.Uint16(content[off : off+2])
	off += 2
	if off+int(minKeyLen) > len(content) {
		return nil, errors.New("truncated min key")
	}
	minKey := make([]byte, minKeyLen)
	copy(minKey, content[off:off+int(minKeyLen)])
	off += int(minKeyLen)

	if off+2 > len(content) {
		return nil, errors.New("truncated max key length")
	}
	maxKeyLen := binary.BigEndian.Uint16(content[off : off+2])
	off += 2
	if off+int(maxKeyLen) > len(content) {
		return nil, errors.New("truncated max key")
	}
	maxKey := make([]byte, maxKeyLen)
	copy(maxKey, content[off:off+int(maxKeyLen)])

	// Load sparse index if present.
	var idx []IndexEntry
	if hdr.IndexSize > 0 {
		idxBuf := make([]byte, hdr.IndexSize)
		if _, err := r.ReadAt(idxBuf, int64(hdr.IndexOffset)); err != nil {
			return nil, errors.Wrap(err, "read index block")
		}
		idx, err = decodeIndex(idxBuf)
		if err != nil {
			return nil, errors.Wrap(err, "decode index")
		}
	}

	// Load bloom filter if present.
	var bloom *BloomFilter
	if hdr.BloomSize > 0 {
		bloomBuf := make([]byte, hdr.BloomSize)
		if _, err := r.ReadAt(bloomBuf, int64(hdr.BloomOffset)); err != nil {
			return nil, errors.Wrap(err, "read bloom filter")
		}
		bloom, err = DecodeBloom(bloomBuf)
		if err != nil {
			return nil, errors.Wrap(err, "decode bloom")
		}
	}

	return &Reader{
		r:      r,
		size:   size,
		header: hdr,
		minKey: minKey,
		maxKey: maxKey,
		index:  idx,
		bloom:  bloom,
	}, nil
}

// Index returns the loaded sparse index entries.
func (rd *Reader) Index() []IndexEntry {
	return rd.index
}

// Bloom returns the loaded bloom filter, or nil if none.
func (rd *Reader) Bloom() *BloomFilter {
	return rd.bloom
}

// Header returns the parsed header.
func (rd *Reader) Header() *Header {
	return rd.header
}

// MinKey returns the smallest key in the SSTable.
func (rd *Reader) MinKey() []byte {
	return rd.minKey
}

// MaxKey returns the largest key in the SSTable.
func (rd *Reader) MaxKey() []byte {
	return rd.maxKey
}

// EntryCount returns the number of entries.
func (rd *Reader) EntryCount() uint32 {
	return rd.header.EntryCount
}

// ReadEntries reads all entries from the data block.
func (rd *Reader) ReadEntries() ([]Entry, error) {
	dataOff := int64(rd.header.DataOffset)
	dataSize := int(rd.header.DataSize)
	buf := make([]byte, dataSize)
	if _, err := rd.r.ReadAt(buf, dataOff); err != nil {
		return nil, errors.Wrap(err, "read data block")
	}
	return decodeDataBlock(buf, rd.header.EntryCount)
}

// Get looks up a key using the sparse index (binary search + window scan).
// Returns the value and true if found, nil and false if not found.
// Tombstoned keys return nil and false.
func (rd *Reader) Get(key []byte) ([]byte, bool, error) {
	keyStr := string(key)

	// Quick range check.
	if keyStr < string(rd.minKey) || keyStr > string(rd.maxKey) {
		return nil, false, nil
	}

	// Bloom filter fast rejection.
	if rd.bloom != nil && !rd.bloom.MayContain(key) {
		return nil, false, nil
	}

	dataSize := rd.header.DataSize

	// Use sparse index to narrow the scan window.
	start, limit := SearchIndex(rd.index, key, dataSize)
	windowSize := limit - start
	window := make([]byte, windowSize)
	if _, err := rd.r.ReadAt(window, int64(rd.header.DataOffset)+int64(start)); err != nil {
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
		entryKey := string(window[off : off+keyLen])
		off += keyLen
		if off+4 > len(window) {
			break
		}
		valLen := binary.BigEndian.Uint32(window[off : off+4])
		off += 4

		if entryKey == keyStr {
			if valLen == TombstoneLen {
				return nil, false, nil
			}
			if off+int(valLen) > len(window) {
				return nil, false, errors.New("truncated value in data window")
			}
			val := make([]byte, valLen)
			copy(val, window[off:off+int(valLen)])
			return val, true, nil
		}
		if entryKey > keyStr {
			return nil, false, nil
		}
		if valLen != TombstoneLen {
			off += int(valLen)
		}
	}
	return nil, false, nil
}

// decodeDataBlock parses entries from the data block bytes.
func decodeDataBlock(buf []byte, count uint32) ([]Entry, error) {
	entries := make([]Entry, 0, count)
	off := 0
	for i := uint32(0); i < count; i++ {
		if off+2 > len(buf) {
			return nil, errors.Errorf("truncated entry %d: key length", i)
		}
		keyLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
		off += 2

		if off+keyLen > len(buf) {
			return nil, errors.Errorf("truncated entry %d: key data", i)
		}
		key := make([]byte, keyLen)
		copy(key, buf[off:off+keyLen])
		off += keyLen

		if off+4 > len(buf) {
			return nil, errors.Errorf("truncated entry %d: value length", i)
		}
		valLen := binary.BigEndian.Uint32(buf[off : off+4])
		off += 4

		if valLen == TombstoneLen {
			entries = append(entries, Entry{Key: key, Tombstone: true})
			continue
		}

		if off+int(valLen) > len(buf) {
			return nil, errors.Errorf("truncated entry %d: value data", i)
		}
		val := make([]byte, valLen)
		copy(val, buf[off:off+int(valLen)])
		off += int(valLen)

		entries = append(entries, Entry{Key: key, Value: val})
	}
	return entries, nil
}
