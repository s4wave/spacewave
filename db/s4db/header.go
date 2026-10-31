package s4db

import (
	"encoding/binary"
	"fmt"
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
	// inlineMax is the largest value stored inline.
	inlineMax uint32
	// chunkSize is the unit of allocation.
	chunkSize uint32
	// blockSize is a run's data block.
	blockSize uint32
	// memtable is the memtable budget in bytes: every process flushes at the
	// same point, whatever its own cache.
	memtable uint64
}

// newHeader returns the header of a new file whose memtable budget is
// memtable bytes.
func newHeader(memtable int64) *header {
	return &header{
		version:   2,
		checksum:  checksumCRC32C,
		index:     indexLogRuns,
		values:    valuesChunked,
		inlineMax: inlineMax,
		chunkSize: chunkSize,
		blockSize: blockSize,
		memtable:  uint64(memtable), // #nosec G115 -- Options bounds it.
	}
}

// decodeHeader reads and validates a header page. A file the previous engine
// wrote fails with ErrPreviousFormat wrapping ErrUnsupported, so a Volume can
// convert it.
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
		chunkSize: binary.LittleEndian.Uint32(b[48:]),
		blockSize: binary.LittleEndian.Uint32(b[52:]),
		memtable:  binary.LittleEndian.Uint64(b[56:]),
	}

	// Recognize the previous engine's files, which convert.
	if h.index == indexLogTree && h.values == valuesPacked {
		return nil, fmt.Errorf("%w: %w", ErrPreviousFormat, ErrUnsupported)
	}

	// Refuse what this version cannot read.
	unsupported := binary.LittleEndian.Uint32(b[12:]) != pageSize ||
		h.incompat&^incompatKnown != 0 ||
		h.rocompat&^rocompatKnown != 0 ||
		h.checksum != checksumCRC32C ||
		h.index != indexLogRuns ||
		h.values != valuesChunked ||
		h.inlineMax != inlineMax ||
		h.chunkSize != chunkSize ||
		h.blockSize != blockSize ||
		h.memtable < minMemtable
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

	// Record the features, algorithms and constants the file uses.
	binary.LittleEndian.PutUint64(b[16:], h.compat)
	binary.LittleEndian.PutUint64(b[24:], h.rocompat)
	binary.LittleEndian.PutUint64(b[32:], h.incompat)
	b[40], b[41], b[42] = h.checksum, h.index, h.values
	binary.LittleEndian.PutUint32(b[44:], h.inlineMax)
	binary.LittleEndian.PutUint32(b[48:], h.chunkSize)
	binary.LittleEndian.PutUint32(b[52:], h.blockSize)
	binary.LittleEndian.PutUint64(b[56:], h.memtable)

	// Seal the page.
	binary.LittleEndian.PutUint32(b[pageSize-4:], checksum(b[:pageSize-4]))
	return b
}
