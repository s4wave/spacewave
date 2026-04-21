package pagestore

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// EncodeLeafPage writes sorted leaf entries into a page buffer.
// Returns the number of entries that fit. Entries must be pre-sorted.
func EncodeLeafPage(buf []byte, entries []LeafEntry) int {
	off := PageHeaderSize
	count := 0
	for i := range entries {
		e := &entries[i]
		needed := LeafEntryOverhead + len(e.Key)
		if e.OverflowPage != 0 {
			needed += 8 // overflow_page_id(4) + overflow_len(4)
		} else {
			needed += len(e.Value)
		}
		if off+needed > len(buf) {
			break
		}
		binary.BigEndian.PutUint16(buf[off:off+2], mustUint16Len(len(e.Key)))
		off += 2
		if e.OverflowPage != 0 {
			binary.BigEndian.PutUint16(buf[off:off+2], OverflowSentinel)
		} else {
			binary.BigEndian.PutUint16(buf[off:off+2], mustUint16Len(len(e.Value)))
		}
		off += 2
		copy(buf[off:], e.Key)
		off += len(e.Key)
		if e.OverflowPage != 0 {
			binary.BigEndian.PutUint32(buf[off:off+4], uint32(e.OverflowPage))
			off += 4
			binary.BigEndian.PutUint32(buf[off:off+4], e.OverflowLen)
			off += 4
		} else {
			copy(buf[off:], e.Value)
			off += len(e.Value)
		}
		count++
	}

	// Write header.
	h := PageHeader{Type: PageTypeLeaf, Count: uint16(count)}
	EncodePageHeader(buf, &h)
	// Compute and store checksum.
	crc := ComputePageChecksum(buf)
	binary.BigEndian.PutUint32(buf[3:7], crc)

	return count
}

// DecodeLeafPage reads entries from a leaf page buffer.
func DecodeLeafPage(buf []byte) ([]LeafEntry, error) {
	if len(buf) < PageHeaderSize {
		return nil, errors.New("leaf page too small")
	}
	h := DecodePageHeader(buf)
	if h.Type != PageTypeLeaf {
		return nil, errors.Errorf("not a leaf page: type=%d", h.Type)
	}

	entries := make([]LeafEntry, 0, h.Count)
	off := PageHeaderSize
	for range h.Count {
		if off+4 > len(buf) {
			return nil, errors.New("truncated leaf entry header")
		}
		keyLen := int(binary.BigEndian.Uint16(buf[off : off+2]))
		valLen := binary.BigEndian.Uint16(buf[off+2 : off+4])
		off += 4

		if off+keyLen > len(buf) {
			return nil, errors.New("truncated leaf key")
		}
		key := make([]byte, keyLen)
		copy(key, buf[off:off+keyLen])
		off += keyLen

		if valLen == OverflowSentinel {
			if off+8 > len(buf) {
				return nil, errors.New("truncated overflow reference")
			}
			pageID := PageID(binary.BigEndian.Uint32(buf[off : off+4]))
			oLen := binary.BigEndian.Uint32(buf[off+4 : off+8])
			off += 8
			entries = append(entries, LeafEntry{
				Key:          key,
				OverflowPage: pageID,
				OverflowLen:  oLen,
			})
		} else {
			vl := int(valLen)
			if off+vl > len(buf) {
				return nil, errors.New("truncated leaf value")
			}
			val := make([]byte, vl)
			copy(val, buf[off:off+vl])
			off += vl
			entries = append(entries, LeafEntry{Key: key, Value: val})
		}
	}
	return entries, nil
}
