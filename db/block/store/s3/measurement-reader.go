package block_store_s3

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ErrMeasurementSourceConfiguration reports a run of missing source objects.
var ErrMeasurementSourceConfiguration = errors.New("measurement configuration error: three consecutive HTTP 404 responses; check bucket and prefix")

// MeasurementReader makes serial range GETs at a fixed rate with no retries.
// Calls are serial; the measuring process owns the reader and its deadline.
// It exposes no write operation. Any failed request ends the run, including
// the first 503, before Client.do can retry it. Use one reader per walk.
type MeasurementReader struct {
	// client signs each request without entering its retry loop.
	client *Client
	// interval is the minimum time between request starts.
	interval time.Duration
	// next is the earliest next start.
	next time.Time
	// missing counts consecutive HTTP 404 responses.
	missing int
	// stopped retains a configuration failure for the process lifetime.
	stopped error
	// Requests counts attempts, including failures.
	Requests uint64
	// Bytes counts response bytes actually consumed.
	Bytes uint64
}

// NewMeasurementReader constructs a reader capped at two requests per second.
func NewMeasurementReader(conf *ClientConfig, interval time.Duration) (*MeasurementReader, error) {
	// Keep the walk below its fixed request ceiling.
	if interval < 500*time.Millisecond {
		return nil, errors.New("measurement requests must be at least 500ms apart")
	}
	client, err := BuildClient(conf)
	if err != nil {
		return nil, err
	}
	client.httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return &MeasurementReader{client: client, interval: interval}, nil
}

// ReadRange reads exactly one bounded range; a server ignoring Range is refused.
func (r *MeasurementReader) ReadRange(ctx context.Context, bucket, key string, off int64, size int) ([]byte, error) {
	// Do not send another request after the missing-object circuit opens.
	if r.stopped != nil {
		return nil, r.stopped
	}

	// Wait for the next request deadline, with no accumulated burst credit.
	timer := time.NewTimer(time.Until(r.next))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case <-timer.C:
	}
	r.next = time.Now().Add(r.interval)
	r.Requests++

	// Send once, without the production client's retry and throttle loop.
	header := http.Header{"Range": {"bytes=" + strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(off+int64(size)-1, 10)}}
	resp, err := r.client.send(ctx, http.MethodGet, bucket, key, nil, nil, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		r.missing++
		if r.missing >= 3 {
			r.stopped = ErrMeasurementSourceConfiguration
			return nil, r.stopped
		}
		return nil, block.ErrNotFound
	}
	r.missing = 0
	if resp.StatusCode == http.StatusRequestedRangeNotSatisfiable {
		return nil, block.ErrNotFound
	}
	if resp.StatusCode != http.StatusPartialContent {
		return nil, errors.Errorf("measurement GET stopped at HTTP %d (no retry)", resp.StatusCode)
	}

	// Count even a partial body, and reject a truncated or oversized answer.
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(size)+1))
	r.Bytes += uint64(len(data))
	if err != nil {
		return nil, err
	}
	if len(data) != size {
		return nil, ErrMeasurementExtent
	}
	return data, nil
}
