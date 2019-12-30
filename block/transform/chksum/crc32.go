package transform_chksum

import (
	"encoding/binary"
	"github.com/pkg/errors"
	"hash/crc32"
)

// DecodeCRC32 is a shortcut for the crc32 checksum transform.
func DecodeCRC32(data []byte) ([]byte, error) {
	if len(data) < 5 {
		return nil, errors.New("short data")
	}
	// get last 4 bytes
	b := data[len(data)-4:]
	data = data[:len(data)-4]
	cs := crc32.ChecksumIEEE(data)
	cse := binary.BigEndian.Uint32(b)
	if cs != cse {
		return nil, errors.Errorf("checksum mismatch %v != %v (indicated)", cs, cse)
	}
	return data, nil
}

// EncodeCRC32 is a shortcut for the crc32 checksum transform.
func EncodeCRC32(data []byte) ([]byte, error) {
	cs := crc32.ChecksumIEEE(data)
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, cs)
	return append(data, b...), nil
}
