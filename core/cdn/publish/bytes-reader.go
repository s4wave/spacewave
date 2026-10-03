package publish

import (
	"io"

	"github.com/pkg/errors"
)

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	// Reject offsets outside the byte slice before copying the requested range.
	if off < 0 {
		return 0, errors.New("negative offset")
	}
	if off >= int64(len(b)) {
		return 0, io.EOF
	}

	// Fill the caller buffer and report EOF when the byte slice ends.
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}
