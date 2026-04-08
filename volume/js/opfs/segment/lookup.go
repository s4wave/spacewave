package segment

import (
	"encoding/binary"
	"io"

	"github.com/pkg/errors"
)

// LookupMeta is the metadata needed for point lookups without reparsing the
// full SSTable on each access.
type LookupMeta struct {
	Header *Header
	MinKey []byte
	MaxKey []byte
	Index  []IndexEntry
	Bloom  *BloomFilter
}

// LoadLookupMeta loads only the SSTable metadata needed for point lookups.
func LoadLookupMeta(r io.ReaderAt, size int64) (*LookupMeta, error) {
	if size < HeaderSize+4 {
		return nil, errors.New("file too small for SSTable")
	}

	var hdrBuf [HeaderSize]byte
	if _, err := r.ReadAt(hdrBuf[:], 0); err != nil {
		return nil, errors.Wrap(err, "read header")
	}
	hdr, err := DecodeHeader(hdrBuf[:])
	if err != nil {
		return nil, errors.Wrap(err, "decode header")
	}

	keyMetaSize := 2 + int(hdr.MinKeySize) + 2 + int(hdr.MaxKeySize)
	keyBuf := make([]byte, keyMetaSize)
	if _, err := r.ReadAt(keyBuf, HeaderSize); err != nil {
		return nil, errors.Wrap(err, "read key metadata")
	}

	off := 0
	minKeyLen := int(binary.BigEndian.Uint16(keyBuf[off : off+2]))
	off += 2
	if off+minKeyLen > len(keyBuf) {
		return nil, errors.New("truncated min key")
	}
	minKey := make([]byte, minKeyLen)
	copy(minKey, keyBuf[off:off+minKeyLen])
	off += minKeyLen

	maxKeyLen := int(binary.BigEndian.Uint16(keyBuf[off : off+2]))
	off += 2
	if off+maxKeyLen > len(keyBuf) {
		return nil, errors.New("truncated max key")
	}
	maxKey := make([]byte, maxKeyLen)
	copy(maxKey, keyBuf[off:off+maxKeyLen])

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

	var bloom *BloomFilter
	if hdr.BloomSize > 0 {
		bloomBuf := make([]byte, hdr.BloomSize)
		if _, err := r.ReadAt(bloomBuf, int64(hdr.BloomOffset)); err != nil {
			return nil, errors.Wrap(err, "read bloom block")
		}
		bloom, err = DecodeBloom(bloomBuf)
		if err != nil {
			return nil, errors.Wrap(err, "decode bloom")
		}
	}

	return &LookupMeta{
		Header: hdr,
		MinKey: minKey,
		MaxKey: maxKey,
		Index:  idx,
		Bloom:  bloom,
	}, nil
}

// Get looks up a key using cached metadata and a single data-window read.
func (m *LookupMeta) Get(r io.ReaderAt, key []byte) ([]byte, bool, error) {
	keyStr := string(key)
	if keyStr < string(m.MinKey) || keyStr > string(m.MaxKey) {
		return nil, false, nil
	}
	if m.Bloom != nil && !m.Bloom.MayContain(key) {
		return nil, false, nil
	}

	start, limit := SearchIndex(m.Index, key, m.Header.DataSize)
	windowSize := int(limit - start)
	window := make([]byte, windowSize)
	if _, err := r.ReadAt(window, int64(m.Header.DataOffset)+int64(start)); err != nil {
		return nil, false, errors.Wrap(err, "read data window")
	}

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
