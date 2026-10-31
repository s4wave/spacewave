package s4dbv1

import (
	"io"

	"github.com/pkg/errors"
)

// liveValueBytes reads the space record sb points at and returns the live
// bytes its value pages hold. A checkpoint with no space record holds no
// values.
func liveValueBytes(r io.ReaderAt, sb superblock) (int64, error) {
	// Read and check the record.
	if sb.space.n == 0 {
		return 0, nil
	}
	b := make([]byte, sb.space.n)
	if _, err := r.ReadAt(b, int64(sb.space.off)); err != nil { // #nosec G115 -- a file offset.
		return 0, err
	}
	if checksum(b) != sb.space.crc {
		return 0, errors.Wrap(ErrCorrupt, "space record checksum mismatch")
	}

	// Skip the release marks, the file end and the free runs.
	d := decoder{b: b}
	d.uvarint()
	d.uvarint()
	d.uvarint()
	for range d.count() {
		d.uvarint()
		d.uvarint()
	}

	// Sum the live runs: each a gap, a page count and the live bytes per
	// page.
	var live int64
	for range d.count() {
		d.uvarint()
		n := d.uvarint()
		live += int64(n * d.uvarint()) // #nosec G115 -- pages times bytes per page.
	}
	if d.err != nil {
		return 0, errors.Wrap(d.err, "space record")
	}
	return live, nil
}
