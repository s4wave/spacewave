package http_range

import (
	"io"
	"net/http"
	"strconv"

	"github.com/pkg/errors"
)

// HttpClient can perform http requests.
type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPRangeReader uses HTTP requests with Range headers to implement
// io.ReadSeeker and io.ReaderAt. It is not concurrency safe.
//
// Note that the body of the request is ignored.
// The method of the request is changed to HEAD for Size().
// Call SetSize to avoid a HEAD request.
type HTTPRangeReader struct {
	client    HttpClient
	request   *http.Request
	seek      int64
	knownSize *int64
}

// NewHTTPRangeReader initializes a HTTPRangeReader for the given request.
func NewHTTPRangeReader(request *http.Request, client HttpClient) *HTTPRangeReader {
	return &HTTPRangeReader{request: request, client: client}
}

// SetSize sets the size of the remote file, avoiding a HEAD request.
func (r *HTTPRangeReader) SetSize(size int64) {
	r.knownSize = &size
}

// ReadAt reads len(buf) bytes into buf starting at offset off.
func (r *HTTPRangeReader) ReadAt(buf []byte, off int64) (int, error) {
	if off < 0 {
		return 0, io.EOF
	}
	length := int64(len(buf))

	req := r.request.Clone(r.request.Context())
	req.Header.Add("Range", fmtRange(off, length))

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, errors.Errorf("unexpected response status: %v", resp.StatusCode)
	}

	n, err := io.ReadFull(resp.Body, buf)
	if err != nil && err != io.ErrUnexpectedEOF && err != io.EOF {
		return n, err
	}
	return n, nil
}

// Read implements the io.Reader interface for HTTPRangeReader.
func (r *HTTPRangeReader) Read(buf []byte) (int, error) {
	n, err := r.ReadAt(buf, r.seek)
	r.seek += int64(n)
	return n, err
}

// Seek sets the offset for the next Read or ReadAt operation.
func (r *HTTPRangeReader) Seek(offset int64, whence int) (int64, error) {
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
func (r *HTTPRangeReader) Size() (int64, error) {
	if r.knownSize != nil {
		return *r.knownSize, nil
	}

	req := r.request.Clone(r.request.Context())
	req.Method = "HEAD"

	resp, err := r.client.Do(req)
	if err != nil {
		return 0, err
	}

	if resp.ContentLength < 0 {
		return 0, errors.New("no content length for Size()")
	}

	r.knownSize = &resp.ContentLength
	return resp.ContentLength, nil
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
