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

// httpTransport issues one browser fetch range request per Fetch call.
type httpTransport struct {
	url         string
	signReq     func(*http.Request) error
	observeResp func(*http.Response)
}

// Fetch reads length bytes starting at off via a browser fetch range request.
// A network error or 5xx response is retried once.
func (t *httpTransport) Fetch(ctx context.Context, off int64, length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
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
	// Build and sign the request.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "build range request")
	}
	req.Header.Set("Range", "bytes="+strconv.FormatInt(off, 10)+"-"+strconv.FormatInt(off+int64(length)-1, 10))
	if t.signReq != nil {
		if err := t.signReq(req); err != nil {
			return nil, errors.Wrap(err, "sign range request")
		}
	}

	// Send it through the browser.
	resp, err := fetch.Fetch(req.URL.String(), &fetch.Opts{Signal: ctx, Header: cloneFetchHeaders(req.Header)})
	if err != nil {
		return nil, &transientError{err: err}
	}
	defer resp.Body.Close()
	if t.observeResp != nil {
		t.observeResp(&http.Response{StatusCode: resp.StatusCode, Header: http.Header(resp.Header)})
	}

	// Read the range, skipping the prefix of a full response.
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
// requests. signReq optionally prepares each request, such as signing it or
// replacing its URL with a granted one.
func NewHTTPRangeReader(
	cli *http.Client,
	url string,
	size int64,
	readAheadSize int,
	signReq func(*http.Request) error,
	observeResp func(*http.Response),
) *PackReader {
	// Build the engine over one transport.
	t := &httpTransport{
		url:         url,
		signReq:     signReq,
		observeResp: observeResp,
	}
	e := NewPackReader(url, size, t)

	// Size its windows for the browser.
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

// _ is a type assertion
var _ Transport = (*httpTransport)(nil)
