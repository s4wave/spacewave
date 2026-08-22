package buffered_reader_at

import (
	"io"
	"sync"
	"testing"
)

// shiftedReader is a SliceReaderAt whose SliceReadAt returns a range
// starting before the requested offset, exercising the offset-shift path.
type shiftedReader struct {
	data []byte
}

// SliceReadAt returns the full buffer starting at offset 0.
func (r *shiftedReader) SliceReadAt(offset, length int64) (int64, []byte, error) {
	return 0, r.data, nil
}

// ReadAt reads from the backing data.
func (r *shiftedReader) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(r.data)) {
		return 0, io.EOF
	}
	n := copy(p, r.data[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// TestConcurrentReadAt exercises concurrent ReadAt calls across shifted
// slice reads and plain ReaderAt paths.
func TestConcurrentReadAt(t *testing.T) {
	data := make([]byte, 64<<10)
	for i := range data {
		data[i] = byte(i)
	}
	br := NewBufferedReaderAt(&shiftedReader{data: data}, 4096)

	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			buf := make([]byte, 512)
			for off := int64(0); off+int64(len(buf)) <= int64(len(data)); off += 512 {
				n, err := br.ReadAt(buf, off)
				if err != nil && err != io.EOF {
					t.Errorf("ReadAt(%d): %v", off, err)
					return
				}
				for i := range n {
					if buf[i] != byte(int(off)+i) {
						t.Errorf("data mismatch at %d: %d != %d", int(off)+i, buf[i], byte(int(off)+i))
						return
					}
				}
			}
		}(w)
	}
	wg.Wait()
}
