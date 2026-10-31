package s4dbv1

import (
	"io"

	"github.com/pkg/errors"
)

// extentRef locates checksummed value bytes in the file.
type extentRef struct {
	// off is the file offset.
	off uint64
	// n is the length.
	n uint32
	// crc is the checksum of the bytes.
	crc uint32
}

// value is a stored value: inline bytes, a reference to an extent, or a
// tombstone in a commit record.
type value struct {
	// inline holds a small value.
	inline []byte
	// ref locates a large value when isRef is set.
	ref extentRef
	// isRef selects ref.
	isRef bool
	// del marks a deleted key.
	del bool
}

// read returns the value's bytes, reading and checking an extent.
func (v value) read(r io.ReaderAt) ([]byte, error) {
	// An inline value is in hand.
	if !v.isRef {
		return v.inline, nil
	}

	// Read and check the extent.
	b := make([]byte, v.ref.n)
	if _, err := r.ReadAt(b, int64(v.ref.off)); err != nil { // #nosec G115 -- a file offset.
		return nil, err
	}
	if checksum(b) != v.ref.crc {
		return nil, errors.Wrapf(ErrCorrupt, "value checksum mismatch at %d", v.ref.off)
	}
	return b, nil
}
