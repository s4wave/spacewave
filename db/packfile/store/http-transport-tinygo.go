//go:build tinygo

package store

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strconv"

	fetch "github.com/aperturerobotics/util/js/fetch"
	"github.com/pkg/errors"
)

const tinyGoPackRangeMaxBytes = 2 * 1024 * 1024

type httpTransport struct {
	url          string
	headers      map[string][]string
	constructErr error
}

// Fetch reads length bytes starting at off via a browser fetch range request.
// A network error or 5xx response is retried once.
func (t *httpTransport) Fetch(ctx context.Context, off int64, length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	if t.constructErr != nil {
		return nil, t.constructErr
	}
	if length > tinyGoPackRangeMaxBytes {
		return nil, errors.Errorf("pack range request length %d exceeds TinyGo browser limit %d", length, tinyGoPackRangeMaxBytes)
	}
	return fetchWithRetry(ctx, func() ([]byte, error) {
		return t.fetchOnce(ctx, off, length)
	})
}

// fetchOnce issues one browser fetch range request.
func (t *httpTransport) fetchOnce(ctx context.Context, off int64, length int) ([]byte, error) {

	req := &fetch.Opts{
		Signal: ctx,
		Header: cloneFetchHeaders(t.headers),
	}
	if req.Header == nil {
		req.Header = make(fetch.Header, 1)
	}
	req.Header.Set("Range", "bytes="+strconv.FormatInt(off, 10)+"-"+strconv.FormatInt(off+int64(length)-1, 10))

	resp, err := fetch.Fetch(t.url, req)
	if err != nil {
		return nil, &transientError{err: err}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusPartialContent:
		return readTinyGoPackRangeBody(resp.Body, length)
	case http.StatusOK:
		if off > 0 {
			if _, err := io.CopyN(io.Discard, resp.Body, off); err != nil {
				if err == io.EOF {
					return nil, nil
				}
				return nil, &transientError{err: errors.Wrap(err, "skipping prefix from full pack response")}
			}
		}
		return readTinyGoPackRangeBody(resp.Body, length)
	case http.StatusRequestedRangeNotSatisfiable:
		return nil, errors.New("requested range not satisfiable")
	case http.StatusForbidden:
		return nil, errors.New("forbidden")
	case http.StatusNotFound:
		return nil, errors.New("not found")
	}
	err = errors.Errorf("unexpected response status: %d", resp.StatusCode)
	if resp.StatusCode >= http.StatusInternalServerError {
		return nil, &transientError{err: err}
	}
	return nil, err
}

func readTinyGoPackRangeBody(r io.Reader, length int) ([]byte, error) {
	buf := make([]byte, length)
	n, err := io.ReadFull(r, buf)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return buf[:n], nil
	}
	if err != nil {
		return nil, &transientError{err: err}
	}
	return buf[:n], nil
}

func cloneFetchHeaders(src map[string][]string) fetch.Header {
	if len(src) == 0 {
		return nil
	}
	dst := make(fetch.Header, len(src))
	for key, vals := range src {
		dst[key] = slices.Clone(vals)
	}
	return dst
}

func (t *httpTransport) SnapshotTransportStats() TransportStats {
	return TransportStats{}
}

// NewHTTPRangeReader builds a per-pack engine backed by browser fetch range
// requests.
func NewHTTPRangeReader(
	cli *http.Client,
	url string,
	size int64,
	readAheadSize int,
	signReq func(*http.Request) error,
	observeResp func(*http.Response),
) *PackReader {
	headers, err := buildSignedRangeHeaders(url, signReq)
	t := &httpTransport{
		url:          url,
		headers:      headers,
		constructErr: err,
	}
	e := NewPackReader(url, size, t)
	if readAheadSize > 0 {
		e.minWindow = readAheadSize
		e.transportQuantum = readAheadSize
		e.currentWindow = readAheadSize
	}
	e.setTransportFetchMaxBytes(tinyGoPackRangeMaxBytes)
	e.budget.limit.Store(16 * 1024 * 1024)
	e.normalizeTransportLocked()
	return e
}

func buildSignedRangeHeaders(url string, signReq func(*http.Request) error) (map[string][]string, error) {
	if signReq == nil {
		return nil, nil
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "build range request")
	}
	if err := signReq(req); err != nil {
		return nil, errors.Wrap(err, "sign range request")
	}
	headers := make(map[string][]string, len(req.Header))
	for key, vals := range req.Header {
		headers[key] = slices.Clone(vals)
	}
	return headers, nil
}

// _ is a type assertion
var _ Transport = (*httpTransport)(nil)
