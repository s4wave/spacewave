package block_store_s3

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// TestMeasurementReaderStopsAtFirstSlowDown proves the tool does not enter the
// normal client's indefinite throttle retry loop or send a write request.
func TestMeasurementReaderStopsAtFirstSlowDown(t *testing.T) {
	// A loopback service refuses the first and only permitted GET with SlowDown.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != http.MethodGet || r.Header.Get("Range") != "bytes=4-7" {
			t.Errorf("unexpected request %s %s", r.Method, r.Header.Get("Range"))
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("SlowDown"))
	}))
	defer srv.Close()
	reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, serialLimits)
	if err != nil {
		t.Fatal(err)
	}

	// A 503 returns immediately, before the context deadline or a second call.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err = reader.ReadRange(ctx, "fixture", "packs/one", 4, 4)
	if err == nil || calls != 1 || reader.Requests.Load() != 1 || ctx.Err() != nil {
		t.Fatalf("error=%v calls=%d requests=%d context=%v", err, calls, reader.Requests.Load(), ctx.Err())
	}
}

// TestMeasurementReaderMissingExtent classifies missing indexed locations
// separately from a throttle or transport failure.
func TestMeasurementReaderMissingExtent(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusRequestedRangeNotSatisfiable} {
		// A loopback fixture exposes only the absent range, with no retry endpoint.
		calls := 0
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			if r.Method != http.MethodGet {
				t.Errorf("unexpected method %s", r.Method)
			}
			w.WriteHeader(status)
		}))
		reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, serialLimits)
		if err != nil {
			srv.Close()
			t.Fatal(err)
		}
		_, err = reader.ReadRange(t.Context(), "fixture", "packs/missing", 4, 4)
		srv.Close()
		if !block.IsNotAvailable(err) || calls != 1 {
			t.Fatalf("HTTP %d: error=%v calls=%d", status, err, calls)
		}
	}
}

// TestMeasurementReaderMissingPrefix stops a consecutive 404 run and sends no fourth request.
func TestMeasurementReaderMissingPrefix(t *testing.T) {
	// Return 404 for every key under an intentionally absent fixture prefix.
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(http.StatusNotFound) }))
	defer srv.Close()
	reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, serialLimits)
	if err != nil {
		t.Fatal(err)
	}

	// Two missing extents are holes; the third is a latched configuration error.
	for i := range 4 {
		_, err = reader.ReadRange(t.Context(), "fixture", "wrong/packs/missing", 0, 1)
		if i < 2 && !block.IsNotAvailable(err) {
			t.Fatalf("request %d: %v", i, err)
		}
		if i >= 2 && (err == nil || !strings.Contains(err.Error(), "configuration error")) {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if calls != 3 {
		t.Fatalf("sent %d requests", calls)
	}
}

// TestMeasurementReaderInvalidExtent distinguishes completed bad ranges from interrupted transport.
func TestMeasurementReaderInvalidExtent(t *testing.T) {
	// A successful but undersized range response is a terminal extent judgment.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte("x"))
	}))
	defer srv.Close()
	reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, serialLimits)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = reader.ReadRange(t.Context(), "fixture", "packs/one", 0, 2); !errors.Is(err, ErrMeasurementExtent) {
		t.Fatalf("short response: %v", err)
	}

	// The real index decoder classifies zero and overflowing extents identically.
	ref, err := block.BuildBlockRef([]byte("extent"), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, extent := range []hashValue{{off: math.MaxUint64, size: 1, extent: true}, {off: 0, size: 0, extent: true}} {
		store := inmem.NewStore()
		tx, err := store.NewTransaction(t.Context(), true)
		if err != nil {
			t.Fatal(err)
		}
		if err = tx.Set(t.Context(), hashRowKey(packfile.BlockKey(ref.GetHash())), extent.marshal()); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(t.Context()); err != nil {
			t.Fatal(err)
		}
		tx.Discard()
		if _, _, _, _, err = LocateMeasurementBlock(t.Context(), store, ref); !errors.Is(err, ErrMeasurementExtent) {
			t.Fatalf("extent %+v: %v", extent, err)
		}
	}
}

// serialLimits is one request at a time at two requests a second.
var serialLimits = MeasurementLimits{Rate: 2, ByteRate: 1 << 20, Inflight: 1}

// TestMeasurementReaderPacesParallelReads holds concurrent callers to the
// lane count and spaces request starts by the slower of the two rates.
func TestMeasurementReaderPacesParallelReads(t *testing.T) {
	// A slow loopback range server records each start and the peak concurrency.
	var mu sync.Mutex
	var starts []time.Time
	active, peak := 0, 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		starts = append(starts, time.Now())
		active++
		peak = max(peak, active)
		mu.Unlock()
		time.Sleep(60 * time.Millisecond)
		mu.Lock()
		active--
		mu.Unlock()
		w.WriteHeader(http.StatusPartialContent)
		w.Write(make([]byte, 100))
	}))
	defer srv.Close()

	// At 30 requests a second and 2000 bytes a second, 100-byte reads start 50ms apart.
	reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, MeasurementLimits{Rate: 30, ByteRate: 2000, Inflight: 2})
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	for range 6 {
		wg.Go(func() {
			if _, err := reader.ReadRange(t.Context(), "fixture", "packs/one", 0, 100); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	if peak != 2 || len(starts) != 6 || reader.Bytes.Load() != 600 {
		t.Fatalf("peak=%d starts=%d bytes=%d", peak, len(starts), reader.Bytes.Load())
	}
	slices.SortFunc(starts, time.Time.Compare)
	for i := 1; i < len(starts); i++ {
		if gap := starts[i].Sub(starts[i-1]); gap < 45*time.Millisecond {
			t.Fatalf("start %d followed the previous after %v", i, gap)
		}
	}
}

// TestMeasurementReaderSlowDownStopsParallelReads latches the first 503:
// callers waiting for their start send nothing after it.
func TestMeasurementReaderSlowDownStopsParallelReads(t *testing.T) {
	// The loopback service refuses every request with SlowDown.
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, MeasurementLimits{Rate: 10, ByteRate: 1 << 20, Inflight: 4})
	if err != nil {
		t.Fatal(err)
	}

	// Eight callers start 100ms apart; the first answer stops the other seven.
	var wg sync.WaitGroup
	var failed atomic.Int32
	for range 8 {
		wg.Go(func() {
			if _, err := reader.ReadRange(t.Context(), "fixture", "packs/one", 0, 4); err != nil && strings.Contains(err.Error(), "HTTP 503") {
				failed.Add(1)
			}
		})
	}
	wg.Wait()
	if calls.Load() != 1 || failed.Load() != 8 || reader.Requests.Load() != 1 {
		t.Fatalf("calls=%d failed=%d requests=%d", calls.Load(), failed.Load(), reader.Requests.Load())
	}
}

// TestMeasurementReaderRefusesUnboundedLimits requires every limit.
func TestMeasurementReaderRefusesUnboundedLimits(t *testing.T) {
	for _, limits := range []MeasurementLimits{
		{Rate: 0, ByteRate: 1, Inflight: 1},
		{Rate: MaxMeasurementRate + 1, ByteRate: 1, Inflight: 1},
		{Rate: 1, ByteRate: 0, Inflight: 1},
		{Rate: 1, ByteRate: 1, Inflight: 0},
		{Rate: 1, ByteRate: 1, Inflight: MaxMeasurementInflight + 1},
	} {
		if _, err := NewMeasurementReader(&ClientConfig{Endpoint: "127.0.0.1:1", DisableSsl: true}, limits); err == nil {
			t.Fatalf("accepted %+v", limits)
		}
	}
}
