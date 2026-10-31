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
