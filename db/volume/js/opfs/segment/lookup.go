package segment

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"strconv"

	trace "github.com/s4wave/spacewave/db/traceutil"

	"github.com/pkg/errors"
)

const maxLookupWindowRead = 256 * 1024

// LookupMeta is the metadata needed for point lookups without reparsing the
// full SSTable on each access.
type LookupMeta struct {
	Header *Header
	MinKey []byte
	MaxKey []byte
	Index  []IndexEntry
	Bloom  *BloomFilter
}

// LookupResult is the result of a batched segment lookup.
type LookupResult struct {
	Value     []byte
	Found     bool
	Tombstone bool
}

// LookupStat is metadata for a lookup result that does not require loading the
// value bytes.
type LookupStat struct {
	ValueSize int64
	Found     bool
	Tombstone bool
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

// Stat resolves a key and returns the value size without materializing the
// value. Tombstoned keys return Found=false with Tombstone=true.
func (m *LookupMeta) Stat(r io.ReaderAt, key []byte) (LookupStat, error) {
	ctx := context.Background()
	_, task := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/stat")
	defer task.End()

	keyStr := string(key)
	if keyStr < string(m.MinKey) || keyStr > string(m.MaxKey) {
		return LookupStat{}, nil
	}
	if m.Bloom != nil && !m.Bloom.MayContain(key) {
		return LookupStat{}, nil
	}

	start, limit := SearchIndex(m.Index, key, m.Header.DataSize)
	if limit < start {
		return LookupStat{}, errors.New("invalid data window")
	}
	windowSize, err := uint32ToInt(limit - start)
	if err != nil {
		return LookupStat{}, err
	}
	if windowSize <= maxLookupWindowRead {
		window := make([]byte, windowSize)
		if _, err := r.ReadAt(window, int64(m.Header.DataOffset)+int64(start)); err != nil {
			return LookupStat{}, errors.Wrap(err, "read data window")
		}
		return statInWindowBytes(window, keyStr)
	}
	return statInWindowReader(r, int64(m.Header.DataOffset), start, limit, key)
}

// Locate resolves a key using cached metadata.
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

	_, subtask := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate/search-index")
	start, limit := SearchIndex(m.Index, key, m.Header.DataSize)
	subtask.End()

	if limit < start {
		return nil, false, false, errors.New("invalid data window")
	}
	windowSize, err := uint32ToInt(limit - start)
	if err != nil {
		return nil, false, false, err
	}
	trace.Log(ctx, "window", "size="+strconv.Itoa(windowSize))

	if windowSize <= maxLookupWindowRead {
		_, subtask = trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate/read-window")
		window := make([]byte, windowSize)
		if _, err := r.ReadAt(window, int64(m.Header.DataOffset)+int64(start)); err != nil {
			subtask.End()
			return nil, false, false, errors.Wrap(err, "read data window")
		}
		subtask.End()

		taskCtx, subtask := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate/scan-window")
		val, found, tombstone, err := locateInWindowBytes(taskCtx, window, keyStr, loadValue)
		subtask.End()
		return val, found, tombstone, err
	}

	_, subtask = trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate/scan-window-streamed")
	val, found, tombstone, err := locateInWindowReader(r, int64(m.Header.DataOffset), start, limit, key, loadValue)
	subtask.End()
	return val, found, tombstone, err
}

func locateInWindowBytes(
	ctx context.Context,
	window []byte,
	keyStr string,
	loadValue bool,
) ([]byte, bool, bool, error) {
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
				return nil, false, true, nil
			}
			if !loadValue {
				return nil, true, false, nil
			}
			if uint64(len(window)-off) < uint64(valLen) {
				return nil, false, false, errors.New("truncated value in data window")
			}
			valLenInt, err := uint32ToInt(valLen)
			if err != nil {
				return nil, false, false, err
			}
			_, copyTask := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate/copy-value")
			val := make([]byte, valLenInt)
			copy(val, window[off:off+valLenInt])
			copyTask.End()
			return val, true, false, nil
		}
		if entryKey > keyStr {
			return nil, false, false, nil
		}
		if valLen != TombstoneLen {
			valLenInt, err := uint32ToInt(valLen)
			if err != nil {
				return nil, false, false, err
			}
			if len(window)-off < valLenInt {
				break
			}
			off += valLenInt
		}
	}
	return nil, false, false, nil
}

func locateInWindowReader(
	r io.ReaderAt,
	dataOffset int64,
	start uint32,
	limit uint32,
	key []byte,
	loadValue bool,
) ([]byte, bool, bool, error) {
	off := start
	var header [4]byte
	for off < limit {
		if limit-off < 2 {
			break
		}
		if _, err := r.ReadAt(header[:2], dataOffset+int64(off)); err != nil {
			return nil, false, false, errors.Wrap(err, "read data window key length")
		}
		keyLen := uint32(binary.BigEndian.Uint16(header[:2]))
		off += 2
		if limit-off < keyLen {
			break
		}

		entryKey := make([]byte, keyLen)
		if keyLen != 0 {
			if _, err := r.ReadAt(entryKey, dataOffset+int64(off)); err != nil {
				return nil, false, false, errors.Wrap(err, "read data window key")
			}
		}
		off += keyLen

		if limit-off < 4 {
			break
		}
		if _, err := r.ReadAt(header[:4], dataOffset+int64(off)); err != nil {
			return nil, false, false, errors.Wrap(err, "read data window value length")
		}
		valLen := binary.BigEndian.Uint32(header[:4])
		off += 4

		cmp := bytes.Compare(entryKey, key)
		if cmp == 0 {
			if valLen == TombstoneLen {
				return nil, false, true, nil
			}
			if !loadValue {
				return nil, true, false, nil
			}
			if uint64(limit-off) < uint64(valLen) {
				return nil, false, false, errors.New("truncated value in data window")
			}
			valLenInt, err := uint32ToInt(valLen)
			if err != nil {
				return nil, false, false, err
			}
			val := make([]byte, valLenInt)
			if valLenInt != 0 {
				if _, err := r.ReadAt(val, dataOffset+int64(off)); err != nil {
					return nil, false, false, errors.Wrap(err, "read data window value")
				}
			}
			return val, true, false, nil
		}
		if cmp > 0 {
			return nil, false, false, nil
		}

		if valLen == TombstoneLen {
			continue
		}
		if uint64(limit-off) < uint64(valLen) {
			break
		}
		off += valLen
	}
	return nil, false, false, nil
}

func statInWindowBytes(window []byte, keyStr string) (LookupStat, error) {
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
				return LookupStat{Tombstone: true}, nil
			}
			if uint64(len(window)-off) < uint64(valLen) {
				return LookupStat{}, errors.New("truncated value in data window")
			}
			return LookupStat{ValueSize: int64(valLen), Found: true}, nil
		}
		if entryKey > keyStr {
			return LookupStat{}, nil
		}
		if valLen != TombstoneLen {
			valLenInt, err := uint32ToInt(valLen)
			if err != nil {
				return LookupStat{}, err
			}
			if len(window)-off < valLenInt {
				break
			}
			off += valLenInt
		}
	}
	return LookupStat{}, nil
}

func statInWindowReader(
	r io.ReaderAt,
	dataOffset int64,
	start uint32,
	limit uint32,
	key []byte,
) (LookupStat, error) {
	off := start
	var header [4]byte
	for off < limit {
		if limit-off < 2 {
			break
		}
		if _, err := r.ReadAt(header[:2], dataOffset+int64(off)); err != nil {
			return LookupStat{}, errors.Wrap(err, "read data window key length")
		}
		keyLen := uint32(binary.BigEndian.Uint16(header[:2]))
		off += 2
		if limit-off < keyLen {
			break
		}

		entryKey := make([]byte, keyLen)
		if keyLen != 0 {
			if _, err := r.ReadAt(entryKey, dataOffset+int64(off)); err != nil {
				return LookupStat{}, errors.Wrap(err, "read data window key")
			}
		}
		off += keyLen

		if limit-off < 4 {
			break
		}
		if _, err := r.ReadAt(header[:4], dataOffset+int64(off)); err != nil {
			return LookupStat{}, errors.Wrap(err, "read data window value length")
		}
		valLen := binary.BigEndian.Uint32(header[:4])
		off += 4

		cmp := bytes.Compare(entryKey, key)
		if cmp == 0 {
			if valLen == TombstoneLen {
				return LookupStat{Tombstone: true}, nil
			}
			if uint64(limit-off) < uint64(valLen) {
				return LookupStat{}, errors.New("truncated value in data window")
			}
			return LookupStat{ValueSize: int64(valLen), Found: true}, nil
		}
		if cmp > 0 {
			return LookupStat{}, nil
		}

		if valLen == TombstoneLen {
			continue
		}
		if uint64(limit-off) < uint64(valLen) {
			break
		}
		off += valLen
	}
	return LookupStat{}, nil
}

// LocateBatch resolves keys using cached metadata and groups keys by
// sparse-index window.
func (m *LookupMeta) LocateBatch(r io.ReaderAt, keys [][]byte, loadValue bool) ([]LookupResult, error) {
	ctx := context.Background()
	ctx, task := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate-batch")
	defer task.End()

	out := make([]LookupResult, len(keys))
	if len(keys) == 0 {
		return out, nil
	}

	type lookupWindow struct {
		start uint32
		limit uint32
		keys  []int
	}
	var windows []lookupWindow
	for i, key := range keys {
		keyStr := string(key)
		if keyStr < string(m.MinKey) || keyStr > string(m.MaxKey) {
			continue
		}
		if m.Bloom != nil && !m.Bloom.MayContain(key) {
			continue
		}

		start, limit := SearchIndex(m.Index, key, m.Header.DataSize)
		var found bool
		for j := range windows {
			if windows[j].start == start && windows[j].limit == limit {
				windows[j].keys = append(windows[j].keys, i)
				found = true
				break
			}
		}
		if !found {
			windows = append(windows, lookupWindow{
				start: start,
				limit: limit,
				keys:  []int{i},
			})
		}
	}

	for _, lw := range windows {
		if lw.limit < lw.start {
			return nil, errors.New("invalid data window")
		}
		windowSize, err := uint32ToInt(lw.limit - lw.start)
		if err != nil {
			return nil, err
		}
		trace.Log(ctx, "window", "size="+strconv.Itoa(windowSize))

		want := make(map[string][]int, len(lw.keys))
		for _, keyIdx := range lw.keys {
			keyStr := string(keys[keyIdx])
			want[keyStr] = append(want[keyStr], keyIdx)
		}

		if windowSize <= maxLookupWindowRead {
			_, subtask := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate-batch/read-window")
			window := make([]byte, windowSize)
			if _, err := r.ReadAt(window, int64(m.Header.DataOffset)+int64(lw.start)); err != nil {
				subtask.End()
				return nil, errors.Wrap(err, "read data window")
			}
			subtask.End()

			_, subtask = trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate-batch/scan-window")
			if err := locateBatchInWindowBytes(window, want, out, loadValue); err != nil {
				subtask.End()
				return nil, err
			}
			subtask.End()
			continue
		}

		_, subtask := trace.NewTask(ctx, "hydra/opfs-segment/lookup-meta/locate-batch/scan-window-streamed")
		if err := locateBatchInWindowReader(r, int64(m.Header.DataOffset), lw.start, lw.limit, want, out, loadValue); err != nil {
			subtask.End()
			return nil, err
		}
		subtask.End()
	}

	return out, nil
}

func locateBatchInWindowBytes(
	window []byte,
	want map[string][]int,
	out []LookupResult,
	loadValue bool,
) error {
	off := 0
	for off < len(window) && len(want) != 0 {
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

		if keyIdxs, ok := want[entryKey]; ok {
			if valLen == TombstoneLen {
				for _, keyIdx := range keyIdxs {
					out[keyIdx].Tombstone = true
				}
				delete(want, entryKey)
				continue
			}
			if loadValue {
				if uint64(len(window)-off) < uint64(valLen) {
					return errors.New("truncated value in data window")
				}
				valLenInt, err := uint32ToInt(valLen)
				if err != nil {
					return err
				}
				for _, keyIdx := range keyIdxs {
					val := make([]byte, valLenInt)
					copy(val, window[off:off+valLenInt])
					out[keyIdx].Value = val
					out[keyIdx].Found = true
				}
			} else {
				for _, keyIdx := range keyIdxs {
					out[keyIdx].Found = true
				}
			}
			delete(want, entryKey)
		}

		if valLen != TombstoneLen {
			valLenInt, err := uint32ToInt(valLen)
			if err != nil {
				return err
			}
			if len(window)-off < valLenInt {
				break
			}
			off += valLenInt
		}
	}
	return nil
}

func locateBatchInWindowReader(
	r io.ReaderAt,
	dataOffset int64,
	start uint32,
	limit uint32,
	want map[string][]int,
	out []LookupResult,
	loadValue bool,
) error {
	off := start
	var header [4]byte
	for off < limit && len(want) != 0 {
		if limit-off < 2 {
			break
		}
		if _, err := r.ReadAt(header[:2], dataOffset+int64(off)); err != nil {
			return errors.Wrap(err, "read data window key length")
		}
		keyLen := uint32(binary.BigEndian.Uint16(header[:2]))
		off += 2
		if limit-off < keyLen {
			break
		}

		entryKey := make([]byte, keyLen)
		if keyLen != 0 {
			if _, err := r.ReadAt(entryKey, dataOffset+int64(off)); err != nil {
				return errors.Wrap(err, "read data window key")
			}
		}
		off += keyLen

		if limit-off < 4 {
			break
		}
		if _, err := r.ReadAt(header[:4], dataOffset+int64(off)); err != nil {
			return errors.Wrap(err, "read data window value length")
		}
		valLen := binary.BigEndian.Uint32(header[:4])
		off += 4

		entryKeyStr := string(entryKey)
		if keyIdxs, ok := want[entryKeyStr]; ok {
			if valLen == TombstoneLen {
				for _, keyIdx := range keyIdxs {
					out[keyIdx].Tombstone = true
				}
				delete(want, entryKeyStr)
				continue
			}
			if loadValue {
				if uint64(limit-off) < uint64(valLen) {
					return errors.New("truncated value in data window")
				}
				valLenInt, err := uint32ToInt(valLen)
				if err != nil {
					return err
				}
				for _, keyIdx := range keyIdxs {
					val := make([]byte, valLenInt)
					if valLenInt != 0 {
						if _, err := r.ReadAt(val, dataOffset+int64(off)); err != nil {
							return errors.Wrap(err, "read data window value")
						}
					}
					out[keyIdx].Value = val
					out[keyIdx].Found = true
				}
			} else {
				for _, keyIdx := range keyIdxs {
					out[keyIdx].Found = true
				}
			}
			delete(want, entryKeyStr)
		}

		if valLen == TombstoneLen {
			continue
		}
		if uint64(limit-off) < uint64(valLen) {
			break
		}
		off += valLen
	}
	return nil
}

func uint32ToInt(v uint32) (int, error) {
	if uint64(v) > uint64(int(^uint(0)>>1)) {
		return 0, errors.New("value length exceeds maximum")
	}
	return int(v), nil
}
