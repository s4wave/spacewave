package segment

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// DefaultIndexInterval is the default number of entries between sparse index entries.
const DefaultIndexInterval = 16

// IndexEntry is a sparse index entry pointing into the data block.
type IndexEntry struct {
	Key        []byte
	DataOffset uint32
}

// encodeIndex serializes sparse index entries.
// Format per entry: [key_len: u16] [key] [data_offset: u32]
func encodeIndex(entries []IndexEntry) []byte {
	size := 0
	for i := range entries {
		size += 2 + len(entries[i].Key) + 4
	}
	buf := make([]byte, size)
	off := 0
	for i := range entries {
		e := &entries[i]
		binary.BigEndian.PutUint16(buf[off:off+2], uint16(len(e.Key)))
		off += 2
		copy(buf[off:], e.Key)
		off += len(e.Key)
		binary.BigEndian.PutUint32(buf[off:off+4], e.DataOffset)
		off += 4
	}
	return buf
}

// decodeIndex parses sparse index entries from buf.
func decodeIndex(buf []byte) ([]IndexEntry, error) {
	var entries []IndexEntry
	off := 0
	for off < len(buf) {
		if off+2 > len(buf) {
			return nil, errors.New("truncated index entry: key length")
		}
		keyLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
		off += 2
		if off+keyLen > len(buf) {
			return nil, errors.New("truncated index entry: key data")
		}
		key := make([]byte, keyLen)
		copy(key, buf[off:off+keyLen])
		off += keyLen
		if off+4 > len(buf) {
			return nil, errors.New("truncated index entry: data offset")
		}
		dataOff := binary.BigEndian.Uint32(buf[off : off+4])
		off += 4
		entries = append(entries, IndexEntry{Key: key, DataOffset: dataOff})
	}
	return entries, nil
}

// searchIndex binary-searches the sparse index for the window containing key.
// Returns the data block byte offset to start scanning from,
// and the byte offset limit (end of the scan window).
// If dataBlockSize is provided, the last window extends to the end of the data block.
func SearchIndex(index []IndexEntry, key []byte, dataBlockSize uint32) (start, limit uint32) {
	if len(index) == 0 {
		return 0, dataBlockSize
	}

	// Binary search for the last index entry <= key.
	lo, hi := 0, len(index)-1
	pos := 0
	for lo <= hi {
		mid := (lo + hi) / 2
		if string(index[mid].Key) <= string(key) {
			pos = mid
			lo = mid + 1
		} else {
			hi = mid - 1
		}
	}

	// If key < first index entry, scan from start of data block.
	if string(key) < string(index[0].Key) {
		start = 0
	} else {
		start = index[pos].DataOffset
	}

	// Limit is the next index entry's offset, or end of data block.
	if pos+1 < len(index) {
		limit = index[pos+1].DataOffset
	} else {
		limit = dataBlockSize
	}
	return start, limit
}
