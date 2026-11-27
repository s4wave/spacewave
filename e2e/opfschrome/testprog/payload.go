//go:build js

package main

import (
	"io"
)

// deterministicLargeBytes builds a reproducible payload from offset zero.
func deterministicLargeBytes(size int, salt int) []byte {
	buf := make([]byte, size)
	fillDeterministicLargeBytes(buf, 0, salt)
	return buf
}

// deterministicLargeWindow builds a reproducible window without earlier payload bytes.
func deterministicLargeWindow(offset, size int, salt int) []byte {
	buf := make([]byte, size)
	fillDeterministicLargeBytes(buf, offset, salt)
	return buf
}

// fillDeterministicLargeBytes fills a window using absolute offsets and a payload salt.
func fillDeterministicLargeBytes(buf []byte, offset int, salt int) {
	for i := range buf {
		buf[i] = deterministicLargeByte(offset+i, salt)
	}
}

// deterministicLargeByte mixes absolute offset and salt into a reproducible byte.
func deterministicLargeByte(offset int, salt int) byte {
	// Mix with a murmur-style finalizer.
	x := uint32(offset) + uint32(0x9e3779b9)
	x ^= uint32(salt) * uint32(0x85ebca6b)
	x ^= x >> 16
	x *= uint32(0x7feb352d)
	x ^= x >> 15
	x *= uint32(0x846ca68b)
	x ^= x >> 16
	return byte(x) + byte(offset)
}

// deterministicLargeReader streams deterministic content with bounded retained state.
type deterministicLargeReader struct {
	// remaining counts unread bytes.
	remaining int
	// offset is the absolute position of the next byte.
	offset int
	// salt selects the reproducible payload.
	salt int
}

// newDeterministicLargeReader constructs a finite deterministic payload stream.
func newDeterministicLargeReader(size int, salt int) *deterministicLargeReader {
	return &deterministicLargeReader{
		remaining: size,
		salt:      salt,
	}
}

// Read fills the caller's buffer until the deterministic stream reaches EOF.
func (r *deterministicLargeReader) Read(p []byte) (int, error) {
	// Report an empty read or EOF.
	if len(p) == 0 {
		return 0, nil
	}
	if r.remaining == 0 {
		return 0, io.EOF
	}

	// Fill the next bytes.
	n := min(len(p), r.remaining)
	for i := range p[:n] {
		p[i] = deterministicLargeByte(r.offset, r.salt)
		r.offset++
	}
	r.remaining -= n
	return n, nil
}
