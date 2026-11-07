package s4dbv1

import (
	"encoding/binary"
)

// checkHeader validates a header page as the previous engine's: its magic,
// checksum, page size, feature flags and algorithms. A file in any other
// format fails with ErrUnsupported.
func checkHeader(b []byte) error {
	// Check the magic and the page checksum.
	if [8]byte(b[:8]) != magic {
		return ErrNotDatabase
	}
	if checksum(b[:pageSize-4]) != binary.LittleEndian.Uint32(b[pageSize-4:]) {
		return ErrCorrupt
	}

	// Accept only what the previous engine wrote.
	unsupported := binary.LittleEndian.Uint32(b[8:]) != headerVersion ||
		binary.LittleEndian.Uint32(b[12:]) != pageSize ||
		binary.LittleEndian.Uint64(b[16:]) != 0 ||
		binary.LittleEndian.Uint64(b[24:]) != 0 ||
		binary.LittleEndian.Uint64(b[32:]) != 0 ||
		b[40] != checksumCRC32C ||
		b[41] != indexLogTree ||
		b[42] != valuesPacked ||
		binary.LittleEndian.Uint32(b[44:]) > maxInline
	if unsupported {
		return ErrUnsupported
	}
	return nil
}
