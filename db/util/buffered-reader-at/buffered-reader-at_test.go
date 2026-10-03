package buffered_reader_at

import (
	"errors"
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
	// Build deterministic data for overlapping reads from a shifted source.
	data := make([]byte, 64<<10)
	for i := range data {
		data[i] = byte(i)
	}
	br := NewBufferedReaderAt(&shiftedReader{data: data}, 4096)

	// Read every data range concurrently and verify the returned bytes.
	var wg sync.WaitGroup
	for w := range 8 {
		wg.Add(1)
		go func(w int) {
			// Verify each buffered range while retaining worker completion tracking.
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

	// Require every concurrent reader to complete before ending the test.
	wg.Wait()
}

type retryReader struct{ attempts int }

func (r *retryReader) ReadAt(p []byte, off int64) (int, error) {
	r.attempts++
	if r.attempts == 1 {
		return 0, errors.New("temporary network error")
	}
	for idx := range p {
		p[idx] = byte(off + int64(idx))
	}
	return len(p), nil
}

func TestReadAtRetriesFailedRange(t *testing.T) {
	// Create a buffered source that fails its first range read.
	source := &retryReader{}
	reader := NewBufferedReaderAt(source, 16)
	buf := make([]byte, 4)

	// Require the first source failure to reach the buffered reader caller.
	if _, err := reader.ReadAt(buf, 3); err == nil {
		t.Fatal("expected first read to fail")
	}

	// Verify the next read reloads the failed range and returns its bytes.
	if n, err := reader.ReadAt(buf, 3); err != nil || n != len(buf) {
		t.Fatalf("retry: n=%d err=%v", n, err)
	}
	if source.attempts != 2 {
		t.Fatalf("attempts=%d, want 2", source.attempts)
	}
	for idx, value := range buf {
		if value != byte(idx+3) {
			t.Fatalf("unexpected range: %v", buf)
		}
	}
}
