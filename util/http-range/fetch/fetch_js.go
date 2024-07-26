//go:build js

package http_range_fetch

import (
	"io"
	"strconv"
	"sync/atomic"

	httplog_fetch "github.com/aperturerobotics/bifrost/http/log/fetch"
	fetch "github.com/aperturerobotics/bifrost/util/js-fetch"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// FetchRangeReader uses fetch requests with Range headers to implement
// io.ReadSeeker and io.ReaderAt. It is concurrency safe.
//
// While Read() and Seek() are concurrency safe, the behavior while using them
// concurrently is undefined. Only use ReadAt concurrently.
//
// Note that the body of the request is ignored.
// The method of the request is changed to HEAD for Size().
// Call SetSize to avoid a HEAD request.
// This type assumes that the URL contents will never change.
// Use hashes in the URL to ensure this.
//
// if le is nil all logging will be disabled
// verbose logs all http responses even if successful
type FetchRangeReader struct {
	le       *logrus.Entry
	fetchUrl string
	opts     *fetch.Opts
	verbose  bool

	seek      atomic.Pointer[int64]
	knownSize atomic.Pointer[uint64]
}

// cachedData contains cached data from the previous request.
type cachedData struct {
	// offset is the location of the read
	offset uint64
	//
	// data is the read data
	data []byte
}

// NewFetchRangeReader initializes a FetchRangeReader for the given request.
// verbose logs http requests
func NewFetchRangeReader(le *logrus.Entry, fetchUrl string, opts *fetch.Opts, verbose bool) *FetchRangeReader {
	return &FetchRangeReader{le: le, fetchUrl: fetchUrl, opts: opts, verbose: verbose}
}

// SetSize sets the size of the remote file, avoiding a HEAD request.
func (r *FetchRangeReader) SetSize(size uint64) {
	r.knownSize.Store(&size)
}

// SliceReadAt reads a slice of data from the requested location.
// NOTE: the returned slice may start before or after the requested location and length.
// NOTE: this may return a completely different range than what you asked for!
func (r *FetchRangeReader) SliceReadAt(offset, length int64) (dataOffset int64, data []byte, err error) {
	if offset < 0 {
		return 0, nil, io.EOF
	}
	if length == 0 {
		return offset, nil, io.ErrShortBuffer
	}

	if knownSizePtr := r.knownSize.Load(); knownSizePtr != nil {
		knownSize := int64(*knownSizePtr)
		if offset >= knownSize {
			return 0, nil, io.EOF
		}
		if offset+length > knownSize {
			length = knownSize - offset
			if length < 0 {
				return 0, nil, io.EOF
			}
		}
	}

	req := r.opts.Clone()
	if req.Header == nil {
		req.Header = make(map[string][]string, 1)
	}
	req.Header.Set("range", fmtRange(offset, length))

	resp, err := httplog_fetch.Fetch(r.le, r.fetchUrl, req, r.verbose)
	if err != nil {
		return 0, nil, err
	}

	switch resp.Status {
	case 200:
		// If the response is 200, the server does not support Range.
		// The entire file was returned, handle that here.
		if int64(len(resp.Body)) < offset+1 {
			return 0, nil, io.EOF
		}
		return 0, resp.Body, io.EOF
	case 206:
		// partial response, as expected.
		return offset, resp.Body, nil
	case 416:
		// Requested Range Not Satisfiable
		return 0, nil, errors.New("requested range not satisfiable")
	case 403:
		// Forbidden
		return 0, nil, errors.New("forbidden")
	case 404:
		// Not Found
		return 0, nil, errors.New("not found")
	default:
		return 0, nil, errors.Errorf("unexpected response status: %d", resp.Status)
	}
}

// ReadAt reads len(buf) bytes into buf starting at offset off.
func (r *FetchRangeReader) ReadAt(buf []byte, off int64) (n int, err error) {
	dataOffset, data, err := r.SliceReadAt(off, int64(len(buf)))
	if err != nil && len(data) == 0 {
		return 0, err
	}
	// Ensure the start index is within the bounds of the data slice.
	start := max(0, off-dataOffset)
	// Ensure the end index does not exceed the length of the data slice.
	end := min(start+int64(len(buf)), int64(len(data)))
	// Copy the data from the calculated start to end index into the buffer.
	n = copy(buf, data[start:end])
	// NOTE: we still return success if n < len(buf) which is not quite what io.ReadAt expects.
	return n, err
}

// Read implements the io.Reader interface for FetchRangeReader.
func (r *FetchRangeReader) Read(buf []byte) (int, error) {
	var seek int64
	seekPtr := r.seek.Load()
	if seekPtr != nil {
		seek = *seekPtr
	}
	n, err := r.ReadAt(buf, seek)
	seek += int64(n)
	r.seek.CompareAndSwap(seekPtr, &seek)
	return n, err
}

// Seek sets the offset for the next Read or ReadAt operation.
func (r *FetchRangeReader) Seek(offset int64, whence int) (int64, error) {
	var seek int64

	switch whence {
	case io.SeekStart:
		seek = offset
	case io.SeekCurrent:
		if seekPtr := r.seek.Load(); seekPtr != nil {
			seek = *seekPtr
		}
		seek += offset
	case io.SeekEnd:
		var length uint64
		if knownSizePtr := r.knownSize.Load(); knownSizePtr != nil {
			length = *knownSizePtr
		} else {
			var err error
			length, err = r.Size()
			if err != nil {
				return 0, err
			}
		}

		seek = int64(length) + offset
	default:
		return 0, errors.New("invalid whence")
	}

	if seek < 0 {
		return 0, errors.New("negative position")
	}

	r.seek.Store(&seek)
	return seek, nil
}

// Size uses an HTTP HEAD request to find out the total available bytes.
func (r *FetchRangeReader) Size() (uint64, error) {
	if knownSizePtr := r.knownSize.Load(); knownSizePtr != nil {
		return *knownSizePtr, nil
	}

	req := r.opts.Clone()
	req.Method = "HEAD"

	resp, err := fetch.Fetch(r.fetchUrl, req)
	if err != nil {
		return 0, err
	}

	switch resp.Status {
	case 200, 206, 204, 304:
		// success case
	case 416:
		// Requested Range Not Satisfiable
		return 0, errors.New("requested range not satisfiable")
	case 403:
		// Forbidden
		return 0, errors.New("forbidden")
	case 404:
		// Not Found
		return 0, errors.New("not found")
	default:
		return 0, errors.Errorf("unexpected response status: %d", resp.Status)
	}

	contentLengthStr := resp.Headers.Get("content-length")
	if len(contentLengthStr) == 0 {
		return 0, errors.New("no content length returned by HEAD request")
	}

	contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "invalid content length header returned by HEAD request")
	}
	if contentLength < 0 {
		return 0, errors.Errorf("invalid negative length header returned by HEAD request: %v", contentLength)
	}

	contentLengthU64 := uint64(contentLength)
	r.knownSize.Store(&contentLengthU64)
	return contentLengthU64, nil
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
