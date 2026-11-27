package block_store_s3

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
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
	reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, time.Second)
	if err != nil {
		t.Fatal(err)
	}

	// A 503 returns immediately, before the context deadline or a second call.
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	_, err = reader.ReadRange(ctx, "fixture", "packs/one", 4, 4)
	if err == nil || calls != 1 || reader.Requests != 1 || ctx.Err() != nil {
		t.Fatalf("error=%v calls=%d requests=%d context=%v", err, calls, reader.Requests, ctx.Err())
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
		reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, time.Second)
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
	reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, 500*time.Millisecond)
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
	reader, err := NewMeasurementReader(&ClientConfig{Endpoint: strings.TrimPrefix(srv.URL, "http://"), DisableSsl: true}, time.Second)
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
