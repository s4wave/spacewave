package block_store_s3

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// ErrMeasurementSourceConfiguration reports a run of missing source objects.
var ErrMeasurementSourceConfiguration = errors.New("measurement configuration error: three consecutive HTTP 404 responses; check bucket and prefix")

// MaxMeasurementRate is the highest request rate a measurement reader accepts.
const MaxMeasurementRate = 30

// MaxMeasurementInflight is the most concurrent requests a measurement reader accepts.
const MaxMeasurementInflight = 32

// MeasurementLimits bounds the load a measurement reader puts on the source
// and on the host's network. Every limit is required.
type MeasurementLimits struct {
	// Rate is the most request starts per second.
	Rate float64
	// ByteRate is the most requested bytes per second, paced at request start.
	ByteRate float64
	// Inflight is the most requests outstanding at once.
	Inflight int
}

// MeasurementReader makes paced range GETs with no retries. Calls may run
// concurrently: each takes one of Inflight lanes, then waits for a start time
// that keeps both the request rate and the requested byte rate under their
// limits, with no accumulated burst credit. It exposes no write operation.
// The first failed HTTP status, such as a 503, latches: no request starts
// after it, and calls already waiting return it. Use one reader per walk.
type MeasurementReader struct {
	// client signs each request without entering its retry loop.
	client *Client
	// gap is the minimum time between request starts.
	gap time.Duration
	// byteRate paces requested bytes per second.
	byteRate float64
	// lanes holds one token per outstanding request.
	lanes chan struct{}

	// mu guards the schedule and the stop state below.
	mu sync.Mutex
	// next is the earliest next start under the request rate.
	next time.Time
	// nextBytes is the earliest next start under the byte rate.
	nextBytes time.Time
	// missing counts consecutive HTTP 404 responses.
	missing int
	// stopped retains a terminal source failure for the process lifetime.
	stopped error

	// Requests counts attempts, including failures.
	Requests atomic.Uint64
	// Bytes counts response bytes actually consumed.
	Bytes atomic.Uint64
}

// NewMeasurementReader constructs a reader under the given limits.
func NewMeasurementReader(conf *ClientConfig, limits MeasurementLimits) (*MeasurementReader, error) {
	// Refuse an unbounded or unapproved load before building a client.
	if !(limits.Rate > 0 && limits.Rate <= MaxMeasurementRate) {
		return nil, errors.Errorf("measurement rate must be above 0 and at most %d requests a second", MaxMeasurementRate)
	}
	if !(limits.ByteRate > 0) {
		return nil, errors.New("measurement byte rate must be above 0")
	}
	if limits.Inflight < 1 || limits.Inflight > MaxMeasurementInflight {
		return nil, errors.Errorf("measurement inflight must be 1 to %d", MaxMeasurementInflight)
	}
	client, err := BuildClient(conf)
	if err != nil {
		return nil, err
	}
	client.httpClient.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }

	// Keep one idle connection per lane, so a request reuses its TLS session.
	if t, ok := client.httpClient.Transport.(*http.Transport); ok {
		t.MaxIdleConnsPerHost = max(t.MaxIdleConnsPerHost, limits.Inflight)
	}
	return &MeasurementReader{
		client:   client,
		gap:      time.Duration(float64(time.Second) / limits.Rate),
		byteRate: limits.ByteRate,
		lanes:    make(chan struct{}, limits.Inflight),
	}, nil
}

// stop returns the latched failure, if any.
func (r *MeasurementReader) stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.stopped
}

// schedule reserves the next start time for a request of size bytes.
func (r *MeasurementReader) schedule(size int) time.Time {
	// Start after now, the previous start's gap and the previous bytes' pace.
	r.mu.Lock()
	defer r.mu.Unlock()
	start := time.Now()
	if start.Before(r.next) {
		start = r.next
	}
	if start.Before(r.nextBytes) {
		start = r.nextBytes
	}
	r.next = start.Add(r.gap)
	r.nextBytes = start.Add(time.Duration(float64(size) / r.byteRate * float64(time.Second)))
	return start
}

// ReadRange reads exactly one bounded range; a server ignoring Range is refused.
func (r *MeasurementReader) ReadRange(ctx context.Context, bucket, key string, off int64, size int) ([]byte, error) {
	// Do not wait for a lane after the source has stopped the run.
	if err := r.stop(); err != nil {
		return nil, err
	}

	// Take a lane first, so a scheduled start is never spent waiting for one.
	select {
	case r.lanes <- struct{}{}:
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	}
	defer func() { <-r.lanes }()

	// Wait for this request's start, then check that no failure arrived meanwhile.
	timer := time.NewTimer(time.Until(r.schedule(size)))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return nil, context.Cause(ctx)
	case <-timer.C:
	}
	if err := r.stop(); err != nil {
		return nil, err
	}
	r.Requests.Add(1)

	// Send once, without the production client's retry and throttle loop.
	header := http.Header{"Range": {"bytes=" + strconv.FormatInt(off, 10) + "-" + strconv.FormatInt(off+int64(size)-1, 10)}}
	resp, err := r.client.send(ctx, http.MethodGet, bucket, key, nil, nil, header)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if err := r.judge(resp.StatusCode); err != nil {
		return nil, err
	}

	// Count even a partial body, and reject a truncated or oversized answer.
	data, err := io.ReadAll(io.LimitReader(resp.Body, int64(size)+1))
	r.Bytes.Add(uint64(len(data)))
	if err != nil {
		return nil, err
	}
	if len(data) != size {
		return nil, ErrMeasurementExtent
	}
	return data, nil
}

// judge classifies a response status: a missing range is a hole, three
// consecutive 404s latch a configuration error, and any other status but 206
// latches the stop for every caller.
func (r *MeasurementReader) judge(status int) error {
	// Count consecutive missing objects; the third latches a configuration stop.
	r.mu.Lock()
	defer r.mu.Unlock()
	if status == http.StatusNotFound {
		r.missing++
		if r.missing >= 3 && r.stopped == nil {
			r.stopped = ErrMeasurementSourceConfiguration
		}
		if r.stopped != nil {
			return r.stopped
		}
		return block.ErrNotFound
	}
	r.missing = 0

	// A missing range is a hole; any other failed status stops every caller.
	if status == http.StatusRequestedRangeNotSatisfiable {
		return block.ErrNotFound
	}
	if status != http.StatusPartialContent {
		if r.stopped == nil {
			r.stopped = errors.Errorf("measurement GET stopped at HTTP %d (no retry)", status)
		}
		return r.stopped
	}
	return nil
}
