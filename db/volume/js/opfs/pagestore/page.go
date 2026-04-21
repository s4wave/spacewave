// Package pagestore implements a B+tree key-value store using fixed-size pages
// with copy-on-write commits and dual superblocks. All I/O through
// io.ReaderAt / io.WriterAt for in-memory testing.
package pagestore

import (
	"encoding/binary"
	"hash/crc32"

	"github.com/pkg/errors"
)

// DefaultPageSize is the default page size in bytes.
const DefaultPageSize = 4096

// PageType identifies the kind of data stored in a page.
type PageType uint8

const (
	PageTypeBranch   PageType = 1
	PageTypeLeaf     PageType = 2
	PageTypeOverflow PageType = 3
	PageTypeFreelist PageType = 4
)

// PageHeaderSize is the fixed page header size.
// Layout: [type: u8] [count: u16] [checksum: u32] [reserved: u8] = 8 bytes
const PageHeaderSize = 8

// PageID is a page number (0-based, maps to file offset = id * pageSize).
type PageID uint32

// InvalidPage is the sentinel for "no page".
const InvalidPage PageID = 0xFFFFFFFF

// PageHeader is the fixed header at the start of every page.
type PageHeader struct {
	Type     PageType
	Count    uint16
	Checksum uint32
}

// EncodePageHeader writes a page header to buf[0:PageHeaderSize].
func EncodePageHeader(buf []byte, h *PageHeader) {
	buf[0] = byte(h.Type)
	binary.BigEndian.PutUint16(buf[1:3], h.Count)
	binary.BigEndian.PutUint32(buf[3:7], h.Checksum)
	buf[7] = 0 // reserved
}

// DecodePageHeader reads a page header from buf[0:PageHeaderSize].
func DecodePageHeader(buf []byte) *PageHeader {
	return &PageHeader{
		Type:     PageType(buf[0]),
		Count:    binary.BigEndian.Uint16(buf[1:3]),
		Checksum: binary.BigEndian.Uint32(buf[3:7]),
	}
}

// ComputePageChecksum computes the CRC32 checksum of a page's body
// (everything after the header, excluding the checksum field itself).
func ComputePageChecksum(page []byte) uint32 {
	// Checksum covers type + count + reserved + body (skip the checksum field).
	var scratch [PageHeaderSize]byte
	copy(scratch[:], page[:PageHeaderSize])
	// Zero the checksum field for computation.
	binary.BigEndian.PutUint32(scratch[3:7], 0)
	crc := crc32.NewIEEE()
	crc.Write(scratch[:])
	crc.Write(page[PageHeaderSize:])
	return crc.Sum32()
}

// ValidatePage checks the page checksum.
func ValidatePage(page []byte) error {
	if len(page) < PageHeaderSize {
		return errors.New("page too small")
	}
	h := DecodePageHeader(page)
	expected := h.Checksum
	actual := ComputePageChecksum(page)
	if expected != actual {
		return errors.Errorf("page checksum mismatch: expected %08x, got %08x", expected, actual)
	}
	return nil
}

// LeafEntry is a key-value entry stored in a leaf page.
// Layout per entry: [key_len: u16] [val_len: u16] [key] [val]
// If val_len == 0xFFFF, the value is stored in overflow pages:
//
//	[key_len: u16] [0xFFFF: u16] [key] [overflow_page_id: u32] [overflow_len: u32]
type LeafEntry struct {
	Key   []byte
	Value []byte
	// Overflow indicates the value is stored in overflow pages.
	OverflowPage PageID
	OverflowLen  uint32
}

// LeafEntryOverhead is the per-entry wire overhead for inline values.
const LeafEntryOverhead = 4 // key_len(2) + val_len(2)

// OverflowSentinel marks an overflow value.
const OverflowSentinel uint16 = 0xFFFF

// BranchEntry is a separator + child pointer in a branch page.
// Layout per entry: [key_len: u16] [child_page_id: u32] [key]
type BranchEntry struct {
	Key     []byte
	ChildID PageID
}

// BranchEntryOverhead is the per-entry wire overhead.
const BranchEntryOverhead = 6 // key_len(2) + child_id(4)
