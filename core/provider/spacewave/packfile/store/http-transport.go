package store

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"sync"

	"github.com/pkg/errors"
	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
)

// httpTransport issues one HTTP range request per Fetch call.
//
// It is the thinnest possible transport: no caching, no in-flight
// deduplication, no adaptive sizing. Those concerns belong to the engine
// that wraps the transport.
type httpTransport struct {
	cli         *http.Client
	url         string
	size        int64
	signReq     func(*http.Request) error
	observeResp func(*http.Response)

	mu                        sync.Mutex
	fullResponseFallbackCount uint64
	fullResponseFallbackBytes int64
	lastFullResponseFallback  int64
}

// Fetch reads length bytes starting at off via HTTP Range.
//
// A 200 OK (non-partial) response is accepted as a best-effort fallback for
// servers that ignore Range; prefix bytes are skipped. Short reads at the
// end of the pack return the partial slice without error so the caller can
// detect EOF by comparing lengths.
func (t *httpTransport) Fetch(ctx context.Context, off int64, length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.url, nil)
	if err != nil {
		return nil, errors.Wrap(err, "build range request")
	}
	end := off + int64(length) - 1
	req.Header.Set("Range", "bytes="+strconv.FormatInt(off, 10)+"-"+strconv.FormatInt(end, 10))
	if t.signReq != nil {
		if err := t.signReq(req); err != nil {
			return nil, errors.Wrap(err, "sign range request")
		}
	}

	resp, err := t.cli.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "range request")
	}
	if t.observeResp != nil {
		t.observeResp(resp)
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(resp)

	if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("range request returned status %d", resp.StatusCode)
	}
	if resp.StatusCode == http.StatusOK && off > 0 {
		if _, err := io.CopyN(io.Discard, resp.Body, off); err != nil {
			if err == io.EOF {
				return nil, nil
			}
			return nil, errors.Wrap(err, "skipping prefix from full-body response")
		}
		t.recordFullResponseFallback(off)
	}

	buf := make([]byte, length)
	n, err := io.ReadFull(resp.Body, buf)
	if err == io.ErrUnexpectedEOF {
		return buf[:n], nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "reading range response")
	}
	return buf[:n], nil
}

func (t *httpTransport) recordFullResponseFallback(bytes int64) {
	if bytes <= 0 {
		return
	}
	t.mu.Lock()
	t.fullResponseFallbackCount++
	t.fullResponseFallbackBytes += bytes
	t.lastFullResponseFallback = bytes
	t.mu.Unlock()
}

func (t *httpTransport) SnapshotTransportStats() TransportStats {
	t.mu.Lock()
	defer t.mu.Unlock()
	return TransportStats{
		FullResponseFallbackCount: t.fullResponseFallbackCount,
		FullResponseFallbackBytes: t.fullResponseFallbackBytes,
		LastFullResponseFallback:  t.lastFullResponseFallback,
	}
}

// NewHTTPRangeReader builds a per-pack engine backed by HTTP range requests.
//
// readAheadSize sets the engine's minimum transport window (and alignment
// quantum). pageSize sets the span store's page size. Either may be zero to
// accept the defaults. signReq optionally mutates the outgoing request
// (for signed CDN access).
func NewHTTPRangeReader(
	cli *http.Client,
	url string,
	size int64,
	readAheadSize int,
	pageSize int,
	signReq func(*http.Request) error,
	observeResp func(*http.Response),
) *PackReader {
	if cli == nil {
		cli = http.DefaultClient
	}
	t := &httpTransport{
		cli:         cli,
		url:         url,
		size:        size,
		signReq:     signReq,
		observeResp: observeResp,
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

// _ is a type assertion
var _ Transport = (*httpTransport)(nil)
