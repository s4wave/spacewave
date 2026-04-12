package segment

import (
	"context"
	"encoding/binary"
	"io"
	"runtime/trace"
	"strconv"

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
	val, found, _, err := m.Locate(r, key, true)
	return val, found, err
}

// Has checks whether a key exists using cached metadata and a single data-window
// read. Tombstoned keys return false.
func (m *LookupMeta) Has(r io.ReaderAt, key []byte) (bool, error) {
	_, found, _, err := m.Locate(r, key, false)
	return found, err
}

// Locate resolves a key using cached metadata and a single data-window read.
// Returns either a live value, a tombstone marker, or a miss.
func (m *LookupMeta) Locate(r io.ReaderAt, key []byte, loadValue bool) ([]byte, bool, bool, error) {
	ctx := context.Background()
	ctx, task := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate")
	defer task.End()

	keyStr := string(key)
	if keyStr < string(m.MinKey) || keyStr > string(m.MaxKey) {
		return nil, false, false, nil
	}
	if m.Bloom != nil && !m.Bloom.MayContain(key) {
		return nil, false, false, nil
	}

	taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate/search-index")
	start, limit := SearchIndex(m.Index, key, m.Header.DataSize)
	subtask.End()
	windowSize := int(limit - start)
	window := make([]byte, windowSize)
	trace.Log(ctx, "window", "size="+strconv.Itoa(windowSize))
	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate/read-window")
	if _, err := r.ReadAt(window, int64(m.Header.DataOffset)+int64(start)); err != nil {
		subtask.End()
		return nil, false, false, errors.Wrap(err, "read data window")
	}
	subtask.End()

	taskCtx, subtask = trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate/scan-window")
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
				subtask.End()
				return nil, false, true, nil
			}
			if !loadValue {
				subtask.End()
				return nil, true, false, nil
			}
			if off+int(valLen) > len(window) {
				subtask.End()
				return nil, false, false, errors.New("truncated value in data window")
			}
			_, copyTask := trace.NewTask(taskCtx, "hydra/opfs-segment/lookup-meta/locate/copy-value")
			val := make([]byte, valLen)
			copy(val, window[off:off+int(valLen)])
			copyTask.End()
			subtask.End()
			return val, true, false, nil
		}
		if entryKey > keyStr {
			subtask.End()
			return nil, false, false, nil
		}
		if valLen != TombstoneLen {
			off += int(valLen)
		}
	}
	subtask.End()
	return nil, false, false, nil
}
