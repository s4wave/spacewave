package block_store_s3

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
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
