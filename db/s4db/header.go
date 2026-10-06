package s4db

import (
	"encoding/binary"
)

// header describes how the file is encoded. It is written once at creation.
type header struct {
	// version is the header layout version.
	version uint32
	// compat lists optional features a reader may ignore.
	compat uint64
	// rocompat lists features a reader must implement to write.
	rocompat uint64
	// incompat lists features a reader must implement to open.
	incompat uint64
	// checksum identifies the checksum algorithm.
	checksum uint8
	// index identifies the index algorithm.
	index uint8
	// values identifies the value storage algorithm.
	values uint8
	// inlineMax is the largest value stored inside the index.
	inlineMax uint32
}

// newHeader returns the header of a new file storing values up to inlineMax
// bytes in the index.
func newHeader(inlineMax int) *header {
	return &header{
		version:   1,
		checksum:  checksumCRC32C,
		index:     indexLogTree,
		values:    valuesPacked,
		inlineMax: uint32(inlineMax), // #nosec G115 -- Options bounds it by maxInline.
	}
}

// decodeHeader reads and validates a header page.
func decodeHeader(b []byte) (*header, error) {
	// Check the magic and the page checksum.
	if [8]byte(b[:8]) != magic {
		return nil, ErrNotDatabase
	}
	if checksum(b[:pageSize-4]) != binary.LittleEndian.Uint32(b[pageSize-4:]) {
		return nil, ErrCorrupt
	}

	// Decode the fields.
	h := &header{
		version:   binary.LittleEndian.Uint32(b[8:]),
		compat:    binary.LittleEndian.Uint64(b[16:]),
		rocompat:  binary.LittleEndian.Uint64(b[24:]),
		incompat:  binary.LittleEndian.Uint64(b[32:]),
		checksum:  b[40],
		index:     b[41],
		values:    b[42],
		inlineMax: binary.LittleEndian.Uint32(b[44:]),
	}

	// Refuse what this version cannot read.
	unsupported := binary.LittleEndian.Uint32(b[12:]) != pageSize ||
		h.incompat&^incompatKnown != 0 ||
		h.rocompat&^rocompatKnown != 0 ||
		h.checksum != checksumCRC32C ||
		h.index != indexLogTree ||
		h.values != valuesPacked ||
		h.inlineMax > maxInline
	if unsupported {
		return nil, ErrUnsupported
	}
	return h, nil
}

// encode writes h into a page.
func (h *header) encode() []byte {
	// Identify the format and its page size.
	b := make([]byte, pageSize)
	copy(b, magic[:])
	binary.LittleEndian.PutUint32(b[8:], h.version)
	binary.LittleEndian.PutUint32(b[12:], pageSize)

	// Name the features and algorithms the file uses.
	binary.LittleEndian.PutUint64(b[16:], h.compat)
	binary.LittleEndian.PutUint64(b[24:], h.rocompat)
	binary.LittleEndian.PutUint64(b[32:], h.incompat)
	b[40], b[41], b[42] = h.checksum, h.index, h.values
	binary.LittleEndian.PutUint32(b[44:], h.inlineMax)

	// Seal the page.
	binary.LittleEndian.PutUint32(b[pageSize-4:], checksum(b[:pageSize-4]))
	return b
}
