package s4db

import (
	"encoding/binary"
)

// A value chunk holds entries from its start: each is a header,
// uvarint(len(key)), the key, uvarint(len(value)) and the CRC-32C of the
// value, followed by the value. A reference points at the value bytes, past
// the header, so reads never parse headers; the cleaner parses them to learn
// each value's key and checks the checksum, stopping at the first entry that
// fails, which can only be the torn tail of a chunk open at a crash. A value
// longer than a chunk takes a run of whole chunks with one entry.

// valueEntryHeaderLen returns the length of the header before a value of n
// bytes stored under a key of k bytes.
func valueEntryHeaderLen(k, n int) int {
	return uvarintLen(uint64(k)) + k + uvarintLen(uint64(n)) + 4 // #nosec G115 -- lengths are not negative.
}

// appendValueEntryHeader appends the header of value under key to b.
func appendValueEntryHeader(b, key, value []byte) []byte {
	b = binary.AppendUvarint(b, uint64(len(key)))
	b = append(b, key...)
	b = binary.AppendUvarint(b, uint64(len(value)))
	return binary.LittleEndian.AppendUint32(b, checksum(value))
}

// valueEntry is one parsed value chunk entry.
type valueEntry struct {
	// key is the entry's key.
	key []byte
	// off is the file offset of the value bytes.
	off uint64
	// n is the value length.
	n uint32
	// data is the value bytes, within the chunk read.
	data []byte
}

// parseValueEntries returns the entries of a value chunk read into b, whose
// first byte is at file offset base, stopping at the first header that does
// not parse or whose value is not whole in b or fails its checksum.
func parseValueEntries(b []byte, base uint64) []valueEntry {
	var entries []valueEntry
	for pos := 0; pos < len(b); {
		// Parse the header.
		d := decoder{b: b[pos:]}
		key := d.bytes()
		n := d.uvarint()
		crc := d.u32()
		if d.err != nil || len(key) == 0 || n > uint64(len(d.b)) {
			break
		}
		at := uint64(len(b) - len(d.b)) // #nosec G115 -- a position in b.
		if checksum(b[at:at+n]) != crc {
			break
		}

		// Record the entry and move past its value.
		entries = append(entries, valueEntry{key: key, off: base + at, n: uint32(n), data: b[at : at+n]}) // #nosec G115 -- n fits a chunk.
		pos = int(at + n)                                                                                 // #nosec G115 -- within b.
	}
	return entries
}
