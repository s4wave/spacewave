package http_range_http

import (
	"io"
	"net/http"
	"strconv"
	"sync/atomic"

	httplog "github.com/aperturerobotics/util/httplog"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// HttpClient can perform http requests.
type HttpClient interface {
	Do(req *http.Request) (*http.Response, error)
}

// HTTPRangeReader uses HTTP requests with Range headers to implement
// io.ReadSeeker and io.ReaderAt. It is concurrency safe.
//
// While Read() and Seek() are concurrency safe, the behavior while using them
// concurrently is undefined. Only use ReadAt concurrently.
//
// Note that the body of the passed request is ignored.
// The method of the request is changed to HEAD for Size().
// Call SetSize to avoid a HEAD request.
//
// if le is nil all logging will be disabled
// verbose logs all http responses even if successful
type HTTPRangeReader struct {
	le      *logrus.Entry
	client  HttpClient
	request *http.Request
	verbose bool

	seek      atomic.Pointer[int64]
	knownSize atomic.Pointer[uint64]
}

// NewHTTPRangeReader initializes a HTTPRangeReader for the given request.
func NewHTTPRangeReader(le *logrus.Entry, request *http.Request, client HttpClient, verbose bool) *HTTPRangeReader {
	return &HTTPRangeReader{le: le, request: request, client: client, verbose: verbose}
}

// SetSize sets the size of the remote file, avoiding a HEAD request.
func (r *HTTPRangeReader) SetSize(size uint64) {
	r.knownSize.Store(&size)
}

// ReadAt reads len(buf) bytes into buf starting at offset off.
func (r *HTTPRangeReader) ReadAt(buf []byte, off int64) (int, error) {
	dataOffset, data, err := r.SliceReadAt(off, int64(len(buf)))
	if err != nil && len(data) == 0 {
		return 0, err
	}
	// Ensure the start index is within the bounds of the data slice.
	start := max(0, off-dataOffset)
	// Ensure the end index does not exceed the length of the data slice.
	end := min(start+int64(len(buf)), int64(len(data)))
	// Copy the data from the calculated start to end index into the buffer.
	n := copy(buf, data[start:end])
	// NOTE: we still return success if n < len(buf) which is not quite what io.ReadAt expects.
	return n, err
}

// SliceReadAt reads a slice of data from the requested location.
// NOTE: the returned slice may start before or after the requested location and length.
// NOTE: this may return a completely different range than what you asked for!
func (r *HTTPRangeReader) SliceReadAt(offset, length int64) (dataOffset int64, data []byte, err error) {
	if offset < 0 {
		return 0, nil, io.EOF
	}
	if length == 0 {
		return offset, nil, io.ErrShortBuffer
	}

	if knownSizePtr := r.knownSize.Load(); knownSizePtr != nil {
		knownSize := int64(*knownSizePtr)
		if offset >= knownSize {
			return offset, nil, io.EOF
		}
		if offset+length > knownSize {
			length = knownSize - offset
			if length < 0 {
				return offset, nil, io.EOF
			}
		}
	}

	req := r.request.Clone(r.request.Context())
	req.Header.Add("Range", fmtRange(offset, length))

	resp, err := httplog.DoRequestWithClient(r.le, r.client, req, r.verbose)
	if err != nil {
		return offset, nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
		// For both OK and Partial Content, read the body.
		// Note: OK means the server does not support Range and returned the entire content.
		var bodyBytes []byte
		var err error
		if resp.StatusCode == http.StatusOK {
			// If the entire file is returned, do not limit the read.
			limitedReader := io.LimitReader(resp.Body, offset+length)
			bodyBytes, err = io.ReadAll(limitedReader)
		} else {
			// For partial content, limit the read to the requested length.
			limitedReader := io.LimitReader(resp.Body, length)
			bodyBytes, err = io.ReadAll(limitedReader)
		}
		if err != nil {
			return 0, nil, errors.Wrap(err, "failed to read response body")
		}
		if resp.StatusCode == http.StatusOK {
			// If the response is 200, the server does not support Range.
			// The entire file was returned, handle that here.
			if int64(len(bodyBytes)) < offset+1 {
				return 0, nil, io.EOF
			}
			return 0, bodyBytes, io.EOF
		} else {
			// Partial content response, as expected.
			return offset, bodyBytes, nil
		}
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
		return 0, nil, errors.Errorf("unexpected response status: %d", resp.StatusCode)
	}
}

// Read implements the io.Reader interface for HTTPRangeReader.
func (r *HTTPRangeReader) Read(buf []byte) (int, error) {
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
func (r *HTTPRangeReader) Seek(offset int64, whence int) (int64, error) {
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

// getSizeFromRequest makes an HTTP request with the specified method and attempts to determine the content length.
// If the Content-Length header is missing in a GET response, it reads the entire body to calculate the size.
func (r *HTTPRangeReader) getSizeFromRequest(method string) (uint64, error) {
	req := r.request.Clone(r.request.Context())
	req.Method = method

	resp, err := httplog.DoRequestWithClient(r.le, r.client, req, r.verbose)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	// Handle response status codes
	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent, http.StatusNoContent, http.StatusNotModified:
		// Success cases
	case http.StatusRequestedRangeNotSatisfiable:
		return 0, errors.New("requested range not satisfiable")
	case http.StatusForbidden:
		return 0, errors.New("forbidden")
	case http.StatusNotFound:
		return 0, errors.New("not found")
	default:
		return 0, errors.Errorf("unexpected response status: %d", resp.StatusCode)
	}

	contentLengthStr := resp.Header.Get("Content-Length")
	if len(contentLengthStr) != 0 {
		contentLength, err := strconv.ParseInt(contentLengthStr, 10, 64)
		if err != nil {
			return 0, errors.Wrap(err, "invalid content length header returned by "+method+" request")
		}
		if contentLength < 0 {
			return 0, errors.Errorf("negative content length returned by "+method+" request: %v", contentLength)
		}
		return uint64(contentLength), nil
	}

	// If Content-Length is missing in a GET response, read the entire body
	if method == http.MethodGet {
		var totalSize uint64
		buffer := make([]byte, 4096) // Buffer for reading in chunks
		for {
			n, err := resp.Body.Read(buffer)
			if n > 0 {
				totalSize += uint64(n)
			}
			if err == io.EOF {
				break
			}
			if err != nil {
				return 0, errors.Wrap(err, "failed to read response body")
			}
		}
		return totalSize, nil
	}

	return 0, errors.New("no content length returned by " + method + " request")
}

// Size uses an HTTP HEAD request to find out the total available bytes.
// If the HEAD request does not return a Content-Length, it attempts a GET request.
// If the GET request also lacks a Content-Length, it reads the entire body to determine the size.
func (r *HTTPRangeReader) Size() (uint64, error) {
	if knownSizePtr := r.knownSize.Load(); knownSizePtr != nil {
		return *knownSizePtr, nil
	}

	// First, try HEAD request
	size, err := r.getSizeFromRequest("HEAD")
	if err != nil {
		if err.Error() == "no content length returned by HEAD request" {
			// Try GET request
			size, err = r.getSizeFromRequest("GET")
			if err != nil {
				return 0, err
			}
		} else {
			return 0, err
		}
	}

	r.knownSize.Store(&size)
	return size, nil
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
