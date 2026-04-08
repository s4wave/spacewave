package pagestore

import (
	"encoding/binary"
	"hash/crc32"

	"github.com/pkg/errors"
)

// SuperblockMagic identifies a superblock.
var SuperblockMagic = [4]byte{'P', 'S', 'S', 'B'}

// SuperblockSize is the fixed superblock size in bytes.
const SuperblockSize = 32

// Superblock describes the root state of the page store at a generation.
type Superblock struct {
	Magic          [4]byte
	Version        uint16
	Generation     uint64
	RootPage       PageID
	FreelistPage   PageID
	PageCount      uint32 // total pages allocated
}

// EncodeSuperblock writes a superblock to buf (32 bytes).
func EncodeSuperblock(buf []byte, sb *Superblock) {
	_ = buf[SuperblockSize-1]
	copy(buf[0:4], sb.Magic[:])
	binary.BigEndian.PutUint16(buf[4:6], sb.Version)
	// buf[6:8] reserved
	binary.BigEndian.PutUint64(buf[8:16], sb.Generation)
	binary.BigEndian.PutUint32(buf[16:20], uint32(sb.RootPage))
	binary.BigEndian.PutUint32(buf[20:24], uint32(sb.FreelistPage))
	binary.BigEndian.PutUint32(buf[24:28], sb.PageCount)
	// CRC32 of first 28 bytes.
	crc := crc32.ChecksumIEEE(buf[:28])
	binary.BigEndian.PutUint32(buf[28:32], crc)
}

// DecodeSuperblock parses a superblock from 32 bytes.
func DecodeSuperblock(buf []byte) (*Superblock, error) {
	if len(buf) < SuperblockSize {
		return nil, errors.New("superblock too short")
	}
	// Verify CRC32.
	expected := binary.BigEndian.Uint32(buf[28:32])
	actual := crc32.ChecksumIEEE(buf[:28])
	if expected != actual {
		return nil, errors.Errorf("superblock CRC32 mismatch: expected %08x, got %08x", expected, actual)
	}
	sb := &Superblock{
		Version:      binary.BigEndian.Uint16(buf[4:6]),
		Generation:   binary.BigEndian.Uint64(buf[8:16]),
		RootPage:     PageID(binary.BigEndian.Uint32(buf[16:20])),
		FreelistPage: PageID(binary.BigEndian.Uint32(buf[20:24])),
		PageCount:    binary.BigEndian.Uint32(buf[24:28]),
	}
	copy(sb.Magic[:], buf[0:4])
	if sb.Magic != SuperblockMagic {
		return nil, errors.Errorf("invalid superblock magic: %x", sb.Magic)
	}
	if sb.Version != 1 {
		return nil, errors.Errorf("unsupported superblock version: %d", sb.Version)
	}
	return sb, nil
}

// PickSuperblock selects the valid superblock with the higher generation.
func PickSuperblock(a, b []byte) *Superblock {
	sa, errA := DecodeSuperblock(a)
	sb, errB := DecodeSuperblock(b)
	if errA != nil && errB != nil {
		return nil
	}
	if errA != nil {
		return sb
	}
	if errB != nil {
		return sa
	}
	if sb.Generation > sa.Generation {
		return sb
	}
	return sa
}
