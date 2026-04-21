package pagestore

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

const freelistNextPageSize = 4

// EncodeFreelistPage writes free page IDs and the next freelist page pointer.
// Returns the number of page IDs written.
func EncodeFreelistPage(buf []byte, nextPage PageID, ids []PageID) int {
	off := PageHeaderSize
	binary.BigEndian.PutUint32(buf[off:off+4], uint32(nextPage))
	off += freelistNextPageSize

	count := 0
	for i := range ids {
		if off+4 > len(buf) {
			break
		}
		binary.BigEndian.PutUint32(buf[off:off+4], uint32(ids[i]))
		off += 4
		count++
	}

	h := PageHeader{Type: PageTypeFreelist, Count: uint16(count)}
	EncodePageHeader(buf, &h)
	crc := ComputePageChecksum(buf)
	binary.BigEndian.PutUint32(buf[3:7], crc)
	return count
}

// DecodeFreelistPage reads the next-page pointer and free page IDs.
func DecodeFreelistPage(buf []byte) (PageID, []PageID, error) {
	if len(buf) < PageHeaderSize+freelistNextPageSize {
		return InvalidPage, nil, errors.New("freelist page too small")
	}
	if err := ValidatePage(buf); err != nil {
		return InvalidPage, nil, err
	}
	h := DecodePageHeader(buf)
	if h.Type != PageTypeFreelist {
		return InvalidPage, nil, errors.Errorf("not a freelist page: type=%d", h.Type)
	}

	off := PageHeaderSize
	nextPage := PageID(binary.BigEndian.Uint32(buf[off : off+4]))
	off += freelistNextPageSize

	ids := make([]PageID, 0, h.Count)
	for range h.Count {
		if off+4 > len(buf) {
			return InvalidPage, nil, errors.New("truncated freelist entry")
		}
		ids = append(ids, PageID(binary.BigEndian.Uint32(buf[off:off+4])))
		off += 4
	}
	return nextPage, ids, nil
}

// FreelistPageCapacity returns the number of page IDs that fit in one freelist page.
func FreelistPageCapacity(pageSize int) int {
	if pageSize <= PageHeaderSize+freelistNextPageSize {
		return 0
	}
	return (pageSize - PageHeaderSize - freelistNextPageSize) / 4
}
