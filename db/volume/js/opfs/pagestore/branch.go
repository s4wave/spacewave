package pagestore

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// EncodeBranchPage writes branch entries into a page buffer.
// The first entry's key may be empty (leftmost child pointer).
// Returns the number of entries that fit.
func EncodeBranchPage(buf []byte, entries []BranchEntry) int {
	off := PageHeaderSize
	count := 0
	for i := range entries {
		e := &entries[i]
		needed := BranchEntryOverhead + len(e.Key)
		if off+needed > len(buf) {
			break
		}
		binary.BigEndian.PutUint16(buf[off:off+2], mustUint16Len(len(e.Key)))
		off += 2
		binary.BigEndian.PutUint32(buf[off:off+4], uint32(e.ChildID))
		off += 4
		copy(buf[off:], e.Key)
		off += len(e.Key)
		count++
	}

	h := PageHeader{Type: PageTypeBranch, Count: mustUint16Len(count)}
	EncodePageHeader(buf, &h)
	crc := ComputePageChecksum(buf)
	binary.BigEndian.PutUint32(buf[3:7], crc)

	return count
}

// DecodeBranchPage reads entries from a branch page buffer.
func DecodeBranchPage(buf []byte) ([]BranchEntry, error) {
	if len(buf) < PageHeaderSize {
		return nil, errors.New("branch page too small")
	}
	h := DecodePageHeader(buf)
	if h.Type != PageTypeBranch {
		return nil, errors.Errorf("not a branch page: type=%d", h.Type)
	}

	entries := make([]BranchEntry, 0, h.Count)
	off := PageHeaderSize
	for range h.Count {
		if off+6 > len(buf) {
			return nil, errors.New("truncated branch entry")
		}
		keyLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
		childID := PageID(binary.BigEndian.Uint32(buf[off+2 : off+6]))
		off += 6

		if off+keyLen > len(buf) {
			return nil, errors.New("truncated branch key")
		}
		key := make([]byte, keyLen)
		copy(key, buf[off:off+keyLen])
		off += keyLen

		entries = append(entries, BranchEntry{Key: key, ChildID: childID})
	}
	return entries, nil
}
