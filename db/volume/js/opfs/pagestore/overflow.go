package pagestore

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

const overflowNextPageSize = 4

// EncodeOverflowPage writes a chained overflow value page.
// Returns the number of value bytes written.
func EncodeOverflowPage(buf []byte, nextPage PageID, value []byte) int {
	off := PageHeaderSize
	binary.BigEndian.PutUint32(buf[off:off+4], uint32(nextPage))
	off += overflowNextPageSize

	n := min(min(len(value), len(buf)-off), int(^uint16(0)))
	copy(buf[off:], value[:n])

	h := PageHeader{Type: PageTypeOverflow, Count: mustUint16Len(n)}
	EncodePageHeader(buf, &h)
	crc := ComputePageChecksum(buf)
	binary.BigEndian.PutUint32(buf[3:7], crc)
	return n
}

// DecodeOverflowPage reads a chained overflow value page.
func DecodeOverflowPage(buf []byte) (PageID, []byte, error) {
	if len(buf) < PageHeaderSize+overflowNextPageSize {
		return InvalidPage, nil, errors.New("overflow page too small")
	}
	if err := ValidatePage(buf); err != nil {
		return InvalidPage, nil, err
	}
	h := DecodePageHeader(buf)
	if h.Type != PageTypeOverflow {
		return InvalidPage, nil, errors.Errorf("not an overflow page: type=%d", h.Type)
	}

	off := PageHeaderSize
	nextPage := PageID(binary.BigEndian.Uint32(buf[off : off+4]))
	off += overflowNextPageSize

	end := off + int(h.Count)
	if end > len(buf) {
		return InvalidPage, nil, errors.New("truncated overflow value")
	}
	value := make([]byte, int(h.Count))
	copy(value, buf[off:end])
	return nextPage, value, nil
}

// OverflowPageCapacity returns the number of value bytes that fit in one overflow page.
func OverflowPageCapacity(pageSize int) int {
	if pageSize <= PageHeaderSize+overflowNextPageSize {
		return 0
	}
	return min(pageSize-PageHeaderSize-overflowNextPageSize, int(^uint16(0)))
}
