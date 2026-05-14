//go:build tinygo

package store

import (
	"context"
	"io"
	"net/http"

	"github.com/pkg/errors"
	http_range "github.com/s4wave/spacewave/db/util/http-range"
)

type httpTransport struct {
	url          string
	size         uint64
	headers      map[string][]string
	constructErr error
}

func (t *httpTransport) Fetch(ctx context.Context, off int64, length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	if t.constructErr != nil {
		return nil, t.constructErr
	}

	reader, err := http_range.NewHTTPRangeReader(ctx, nil, t.url, t.headers, false, false)
	if err != nil {
		return nil, err
	}
	reader.SetSize(t.size)

	buf := make([]byte, length)
	n, err := reader.ReadAt(buf, off)
	if err == io.EOF || err == io.ErrUnexpectedEOF {
		return buf[:n], nil
	}
	if err != nil {
		return nil, err
	}
	return buf[:n], nil
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
	pageSize int,
	signReq func(*http.Request) error,
	observeResp func(*http.Response),
) *PackReader {
	headers, err := buildSignedRangeHeaders(url, signReq)
	t := &httpTransport{
		url:          url,
		size:         uint64(size),
		headers:      headers,
		constructErr: err,
	}
	e := NewPackReader(url, size, t, 0)
	if readAheadSize > 0 {
		e.minWindow = readAheadSize
		e.transportQuantum = readAheadSize
		e.currentWindow = readAheadSize
	}
	if pageSize > 0 {
		e.pageSize = pageSize
	}
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
		headers[key] = append([]string(nil), vals...)
	}
	return headers, nil
}

// _ is a type assertion
var _ Transport = (*httpTransport)(nil)
