package s4db

import "encoding/binary"

// valueWrite is a large value awaiting its write.
type valueWrite struct {
	// op indexes the record op holding the value's reference.
	op int
	// entry is the file offset of the value's header.
	entry uint64
	// data is the value.
	data []byte
}

// A value chunk holds a sequence of entries from its start: each entry is a
// header, uvarint(len(key)+1), the key, and uvarint(len(value)), followed by
// the value. A zero byte where a header would start ends the chunk's entries.
// A value's reference points at its bytes, past the header, so reads never
// parse headers; the cleaner reads them to find each value's key.

// valueHeaderLen returns the length of the header before a value of n bytes
// stored under a key of k bytes.
func valueHeaderLen(k, n int) int {
	return uvarintLen(uint64(k)+1) + k + uvarintLen(uint64(n)) // #nosec G115 -- lengths are not negative.
}

// appendValueHeader appends the header of value under key to b.
func appendValueHeader(b, key, value []byte) []byte {
	b = binary.AppendUvarint(b, uint64(len(key))+1)
	b = append(b, key...)
	return binary.AppendUvarint(b, uint64(len(value)))
}
