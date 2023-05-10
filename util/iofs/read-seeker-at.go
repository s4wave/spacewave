package iofs

import (
	"io"
	"sync"
)

// ReadSeekerAt implements ReadAt with a ReadSeeker.
//
// Note: this prevents concurrent read calls, use only if absolutely necessary.
type ReadSeekerAt struct {
	rs  io.ReadSeeker
	mtx sync.Mutex
}

// NewReadSeekerAt constructs a new ReadSeekerAt from a ReadSeeker.
func NewReadSeekerAt(rs io.ReadSeeker) *ReadSeekerAt {
	return &ReadSeekerAt{rs: rs}
}

// Read reads up to len(p) bytes into p. It returns the number of bytes
// read (0 <= n <= len(p)) and any error encountered. Even if Read
// returns n < len(p), it may use all of p as scratch space during the call.
// If some data is available but not len(p) bytes, Read conventionally
// returns what is available instead of waiting for more.
func (r *ReadSeekerAt) Read(p []byte) (n int, err error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	return r.rs.Read(p)
}

// Seeker is the interface that wraps the basic Seek method.
//
// Seek sets the offset for the next Read or Write to offset,
// interpreted according to whence:
// SeekStart means relative to the start of the file,
// SeekCurrent means relative to the current offset, and
// SeekEnd means relative to the end
// (for example, offset = -2 specifies the penultimate byte of the file).
// Seek returns the new offset relative to the start of the
// file or an error, if any.
//
// Seeking to an offset before the start of the file is an error.
// Seeking to any positive offset may be allowed, but if the new offset exceeds
// the size of the underlying object the behavior of subsequent I/O operations
// is implementation-dependent.
func (r *ReadSeekerAt) Seek(offset int64, whence int) (int64, error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	return r.rs.Seek(offset, whence)
}

// ReaderAt is the interface that wraps the basic ReadAt method.
//
// ReadAt reads len(p) bytes into p starting at offset off in the
// underlying input source. It returns the number of bytes
// read (0 <= n <= len(p)) and any error encountered.
//
// When ReadAt returns n < len(p), it returns a non-nil error
// explaining why more bytes were not returned. In this respect,
// ReadAt is stricter than Read.
//
// Even if ReadAt returns n < len(p), it may use all of p as scratch
// space during the call. If some data is available but not len(p) bytes,
// ReadAt blocks until either all the data is available or an error occurs.
// In this respect ReadAt is different from Read.
//
// If the n = len(p) bytes returned by ReadAt are at the end of the
// input source, ReadAt may return either err == EOF or err == nil.
//
// If ReadAt is reading from an input source with a seek offset,
// ReadAt should not affect nor be affected by the underlying
// seek offset.
//
// Clients of ReadAt can execute parallel ReadAt calls on the
// same input source.
//
// Implementations must not retain p.
func (r *ReadSeekerAt) ReadAt(p []byte, off int64) (n int, err error) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	startPos, err := r.rs.Seek(0, io.SeekCurrent)
	if err != nil {
		return 0, err
	}
	defer func() {
		_, errSeek := r.rs.Seek(startPos, io.SeekStart)
		if err == nil {
			err = errSeek
		}
	}()

	_, err = r.rs.Seek(off, io.SeekStart)
	if err != nil {
		return 0, err
	}

	return r.rs.Read(p)
}

// _ is a type assertion
var (
	_ io.ReaderAt   = ((*ReadSeekerAt)(nil))
	_ io.ReadSeeker = ((*ReadSeekerAt)(nil))
)
