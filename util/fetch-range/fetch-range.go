//go:build js

package fetch_range

import (
	"io"
	"strconv"

	fetch "github.com/aperturerobotics/bldr/util/wasm-fetch"
	"github.com/pkg/errors"
)

// FetchRangeReader uses fetch requests with Range headers to implement
// io.ReadSeeker and io.ReaderAt. It is not concurrency safe.
//
// Note that the body of the request is ignored.
// The method of the request is changed to HEAD for Size().
// Call SetSize to avoid a HEAD request.
type FetchRangeReader struct {
	fetchUrl  string
	opts      *fetch.Opts
	seek      int64
	knownSize *int64
}

// NewFetchRangeReader initializes a FetchRangeReader for the given request.
func NewFetchRangeReader(fetchUrl string, opts *fetch.Opts) *FetchRangeReader {
	return &FetchRangeReader{fetchUrl: fetchUrl, opts: opts}
}

// SetSize sets the size of the remote file, avoiding a HEAD request.
func (r *FetchRangeReader) SetSize(size int64) {
	r.knownSize = &size
}

// ReadAt reads len(buf) bytes into buf starting at offset off.
func (r *FetchRangeReader) ReadAt(buf []byte, off int64) (int, error) {
	if off < 0 {
		return 0, io.EOF
	}
	length := int64(len(buf))

	req := r.opts.Clone()
	if req.Headers == nil {
		req.Headers = make(map[string]string, 1)
	}
	req.Headers["Range"] = fmtRange(off, length)

	resp, err := fetch.Fetch(r.fetchUrl, req)
	if err != nil {
		return 0, err
	}

	if resp.Status != 200 && resp.Status != 206 {
		return 0, errors.Errorf("unexpected response status: %v", resp.Status)
	}

	n := copy(buf, resp.Body)
	return n, nil
}

// Read implements the io.Reader interface for FetchRangeReader.
func (r *FetchRangeReader) Read(buf []byte) (int, error) {
	n, err := r.ReadAt(buf, r.seek)
	r.seek += int64(n)
	return n, err
}

// Seek sets the offset for the next Read or ReadAt operation.
func (r *FetchRangeReader) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
		r.seek = offset
	case io.SeekCurrent:
		r.seek += offset
	case io.SeekEnd:
		var length int64
		if r.knownSize != nil {
			length = *r.knownSize
		} else {
			var err error
			length, err = r.Size()
			if err != nil {
				return 0, err
			}
		}

		r.seek = length + offset
	default:
		return 0, errors.New("invalid whence")
	}

	if r.seek < 0 {
		return 0, errors.New("negative position")
	}
	return r.seek, nil
}

// Size uses an HTTP HEAD request to find out the total available bytes.
func (r *FetchRangeReader) Size() (int64, error) {
	if r.knownSize != nil {
		return *r.knownSize, nil
	}

	req := r.opts.Clone()
	req.Method = "HEAD"

	resp, err := fetch.Fetch(r.fetchUrl, req)
	if err != nil {
		return 0, err
	}

	contentLengthStr := resp.Headers.Get("content-length")
	if len(contentLengthStr) == 0 {
		return 0, errors.New("no content length returned by HEAD request")
	}

	contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "invalid content length header returned by HEAD request")
	}

	r.knownSize = &contentLength
	return int64(contentLength), nil
}

func fmtRange(from, length int64) string {
	var to int64
	if length > 0 {
		to = from + length - 1
	} else {
		to = from
	}
	return "bytes=" + strconv.FormatInt(from, 10) + "-" + strconv.FormatInt(to, 10)
}
