// Package segment implements a sorted-string table (SSTable) binary format
// for key-value storage. The format is platform-independent and operates
// through io.Writer / io.ReaderAt interfaces.
//
// SSTable layout:
//
//	+-------------------------------------------+
//	| Header (64 bytes fixed)                   |
//	+-------------------------------------------+
//	| Min Key (variable)                        |
//	+-------------------------------------------+
//	| Max Key (variable)                        |
//	+-------------------------------------------+
//	| Data Block (sorted key-value entries)      |
//	+-------------------------------------------+
//	| Index Block (sparse index, every Nth key) |
//	+-------------------------------------------+
//	| Bloom Filter                              |
//	+-------------------------------------------+
//	| Footer (CRC32, 4 bytes)                   |
//	+-------------------------------------------+
package segment

import (
	"encoding/binary"

	"github.com/pkg/errors"
)

// Magic is the 4-byte file signature.
var Magic = [4]byte{'O', 'S', 'S', 'T'}

// HeaderSize is the fixed header size in bytes.
const HeaderSize = 64

// CurrentVersion is the format version.
const CurrentVersion uint16 = 1

// TombstoneLen is the reserved val_len sentinel for deleted keys.
const TombstoneLen uint32 = 0xFFFFFFFF

// Header is the fixed-size file header.
type Header struct {
	Magic       [4]byte
	Version     uint16
	Flags       uint16
	EntryCount  uint32
	DataOffset  uint32
	DataSize    uint32
	IndexOffset uint32
	IndexSize   uint32
	BloomOffset uint32
	BloomSize   uint32
	MinKeySize  uint16
	MaxKeySize  uint16
	// 28 bytes reserved (zero-filled).
}

// Encode writes the header into a 64-byte buffer.
func (h *Header) Encode(buf []byte) {
	_ = buf[HeaderSize-1]
	copy(buf[0:4], h.Magic[:])
	binary.BigEndian.PutUint16(buf[4:6], h.Version)
	binary.BigEndian.PutUint16(buf[6:8], h.Flags)
	binary.BigEndian.PutUint32(buf[8:12], h.EntryCount)
	binary.BigEndian.PutUint32(buf[12:16], h.DataOffset)
	binary.BigEndian.PutUint32(buf[16:20], h.DataSize)
	binary.BigEndian.PutUint32(buf[20:24], h.IndexOffset)
	binary.BigEndian.PutUint32(buf[24:28], h.IndexSize)
	binary.BigEndian.PutUint32(buf[28:32], h.BloomOffset)
	binary.BigEndian.PutUint32(buf[32:36], h.BloomSize)
	binary.BigEndian.PutUint16(buf[36:38], h.MinKeySize)
	binary.BigEndian.PutUint16(buf[38:40], h.MaxKeySize)
	// Zero the reserved area.
	for i := 40; i < HeaderSize; i++ {
		buf[i] = 0
	}
}

// DecodeHeader parses a 64-byte header from buf.
func DecodeHeader(buf []byte) (*Header, error) {
	if len(buf) < HeaderSize {
		return nil, errors.New("header too short")
	}
	h := &Header{
		Version:     binary.BigEndian.Uint16(buf[4:6]),
		Flags:       binary.BigEndian.Uint16(buf[6:8]),
		EntryCount:  binary.BigEndian.Uint32(buf[8:12]),
		DataOffset:  binary.BigEndian.Uint32(buf[12:16]),
		DataSize:    binary.BigEndian.Uint32(buf[16:20]),
		IndexOffset: binary.BigEndian.Uint32(buf[20:24]),
		IndexSize:   binary.BigEndian.Uint32(buf[24:28]),
		BloomOffset: binary.BigEndian.Uint32(buf[28:32]),
		BloomSize:   binary.BigEndian.Uint32(buf[32:36]),
		MinKeySize:  binary.BigEndian.Uint16(buf[36:38]),
		MaxKeySize:  binary.BigEndian.Uint16(buf[38:40]),
	}
	copy(h.Magic[:], buf[0:4])
	if h.Magic != Magic {
		return nil, errors.Errorf("invalid magic: %x", h.Magic)
	}
	if h.Version != CurrentVersion {
		return nil, errors.Errorf("unsupported version: %d", h.Version)
	}
	return h, nil
}

// Entry is a key-value pair in the data block.
type Entry struct {
	Key   []byte
	Value []byte
	// Tombstone indicates this entry is a deletion marker.
	Tombstone bool
}

// EntryOverhead is the per-entry wire overhead: key_len(2) + val_len(4).
const EntryOverhead = 6
