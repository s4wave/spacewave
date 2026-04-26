//go:build !tinygo

package store

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

func TestHTTPRangeReaderDefaults(t *testing.T) {
	rd := NewHTTPRangeReader(nil, "https://example.com/pack", 1024, 0, 0, nil, nil)
	if rd.maxBytes != defaultResidentBudget {
		t.Fatalf("maxBytes = %d, want %d", rd.maxBytes, defaultResidentBudget)
	}
	if rd.maxWindow != defaultTransportMaxWindow {
		t.Fatalf("maxWindow = %d, want %d", rd.maxWindow, defaultTransportMaxWindow)
	}
	if rd.minWindow != defaultTransportMinWindow {
		t.Fatalf("minWindow = %d, want %d", rd.minWindow, defaultTransportMinWindow)
	}
	if rd.currentWindow != defaultTransportMinWindow {
		t.Fatalf("currentWindow = %d, want %d", rd.currentWindow, defaultTransportMinWindow)
	}
	if rd.transportQuantum != defaultTransportMinWindow {
		t.Fatalf("transportQuantum = %d, want %d", rd.transportQuantum, defaultTransportMinWindow)
	}

	rd = NewHTTPRangeReader(nil, "https://example.com/pack", 1024, 16, 4, nil, nil)
	if rd.minWindow != 16 {
		t.Fatalf("minWindow = %d, want 16", rd.minWindow)
	}
	if rd.transportQuantum != 16 {
		t.Fatalf("transportQuantum = %d, want 16", rd.transportQuantum)
	}
	if rd.currentWindow != 16 {
		t.Fatalf("currentWindow = %d, want 16", rd.currentWindow)
	}
	if rd.pageSize != 4 {
		t.Fatalf("pageSize = %d, want 4", rd.pageSize)
	}
}

func TestPackReaderPlanFetchLeftShiftsWithinGap(t *testing.T) {
	eng := NewPackReader("shift-pack", 8<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}), 0)
	eng.minWindow = 1 << 20
	eng.transportQuantum = 1 << 20
	eng.maxWindow = 2 << 20
	eng.currentWindow = 2 << 20
	eng.sparseReads = false
	eng.spans = []*span{{off: 3 << 20, size: 1 << 20}}

	key := eng.planFetchLocked(5<<19, (5<<19)+1)
	if key.off != 1<<20 || key.size != 2<<20 {
		t.Fatalf("planFetchLocked() = [%d,%d), want [%d,%d)", key.off, key.end(), int64(1<<20), int64(3<<20))
	}
}

func TestPackReaderSparsePlanCapsColdBackshift(t *testing.T) {
	eng := NewPackReader("sparse-shift-pack", 10<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}), 0)
	eng.minWindow = 256 << 10
	eng.transportQuantum = 256 << 10
	eng.maxWindow = 8 << 20
	eng.currentWindow = 8 << 20
	eng.sparseReads = true
	eng.sparseColdWindow = 256 << 10
	eng.sparseLocalityDistance = 512 << 10
	eng.spans = []*span{{off: 8 << 20, size: 256 << 10}}

	key := eng.planFetchLocked(5<<20, (5<<20)+1)
	if key.off != 5<<20 || key.size != 256<<10 {
		t.Fatalf("sparse planFetchLocked() = [%d,%d), want [%d,%d)", key.off, key.end(), int64(5<<20), int64((5<<20)+(256<<10)))
	}
}

func TestPackReaderSparsePlanPromotesNearbyReads(t *testing.T) {
	eng := NewPackReader("sparse-local-pack", 10<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}), 0)
	eng.minWindow = 256 << 10
	eng.transportQuantum = 256 << 10
	eng.maxWindow = 2 << 20
	eng.currentWindow = 2 << 20
	eng.sparseReads = true
	eng.sparseColdWindow = 256 << 10
	eng.sparseLocalityDistance = 512 << 10

	first := eng.planFetchLocked(1<<20, (1<<20)+1)
	if first.size != 256<<10 {
		t.Fatalf("first sparse fetch size = %d, want %d", first.size, 256<<10)
	}
	second := eng.planFetchLocked((1<<20)+(128<<10), (1<<20)+(128<<10)+1)
	if second.size <= first.size {
		t.Fatalf("nearby sparse fetch size = %d, want promotion above %d", second.size, first.size)
	}
}

func TestPackReaderPlanFetchShrinksWhenCoveredOnBothSides(t *testing.T) {
	eng := NewPackReader("shrink-pack", 8<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}), 0)
	eng.minWindow = 1 << 20
	eng.transportQuantum = 1 << 20
	eng.maxWindow = 2 << 20
	eng.currentWindow = 2 << 20
	eng.spans = []*span{
		{off: 0, size: 1 << 20},
		{off: 2 << 20, size: 1 << 20},
	}

	key := eng.planFetchLocked(3<<19, (3<<19)+1)
	if key.off != 1<<20 || key.size != 1<<20 {
		t.Fatalf("planFetchLocked() = [%d,%d), want [%d,%d)", key.off, key.end(), int64(1<<20), int64(2<<20))
	}
}

func TestPackReaderSnapshotStats(t *testing.T) {
	eng := NewPackReader("stats-pack", 1024, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}), 0)
	eng.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		eng.currentWindow = 256
		eng.loading = map[fetchKey]*fetchLoad{
			{off: 0, size: 8}: {done: make(chan struct{})},
		}
		eng.spans = []*span{
			{off: 0, size: 8, pins: 1},
			{off: 8, size: 8},
		}
		eng.residentBytes = 16
		eng.blocks = map[string]*blockRecord{
			"verifying": {state: blockStateVerifying},
			"published": {state: blockStatePublished},
		}
		eng.verifyQueued = 2
		eng.verifyRunning = 1
		eng.verifyCompleted = 3
		eng.verifyFailures = 1
		eng.writebackCount = 4
		eng.writebackErrors = 1
		eng.indexLoaded = true
		eng.recordFetchLocked(fetchKey{off: 0, size: 64}, 48)
	})

	stats := eng.SnapshotStats()
	if stats.ResidentBytes != 16 || stats.PinnedBytes != 8 {
		t.Fatalf("unexpected resident stats: %+v", stats)
	}
	if stats.FetchCount != 1 || stats.FetchedBytes != 64 || stats.LastFetchBytes != 64 {
		t.Fatalf("unexpected fetch stats: %+v", stats)
	}
	if stats.RangeRequestCount != 1 || stats.RangeResponseBytes != 48 {
		t.Fatalf("unexpected range response stats: %+v", stats)
	}
	if stats.BlockCount != 2 || stats.VerifyingBlocks != 1 || stats.PublishedBlocks != 1 {
		t.Fatalf("unexpected block stats: %+v", stats)
	}
	if stats.VerifyQueued != 2 || stats.VerifyRunning != 1 || stats.VerifyCompleted != 3 {
		t.Fatalf("unexpected verify stats: %+v", stats)
	}
	if stats.WritebackCount != 4 || stats.WritebackErrors != 1 || !stats.IndexLoaded {
		t.Fatalf("unexpected publication stats: %+v", stats)
	}
}

func TestPackfileStoreAppliesTuningOverrides(t *testing.T) {
	store := NewPackfileStore(func(packID string, size int64) (*PackReader, error) {
		return NewPackReader(packID, size, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
			return nil, nil
		}), 0), nil
	}, newMemIndexCache())
	store.SetTransportPageSize(8)
	store.SetTransportMinWindow(32)
	store.SetTransportQuantum(64)
	store.SetTransportMaxWindow(256)
	store.SetTransportTargetRequestHz(2)
	store.SetTransportWindowSmoothing(0.5)
	store.SetIndexPromotionEnabled(false)

	eng, err := store.getOrOpenEngine("cfg-pack", 4096, 0)
	if err != nil {
		t.Fatalf("getOrOpenEngine: %v", err)
	}
	tuning := eng.SnapshotTuning()
	if tuning.PageSize != 8 || tuning.MinWindow != 32 || tuning.TransportQuantum != 64 {
		t.Fatalf("unexpected page/min/quantum tuning: %+v", tuning)
	}
	if tuning.MaxWindow != 256 || tuning.TargetRequestHz != 2 || tuning.Smoothing != 0.5 {
		t.Fatalf("unexpected transport tuning: %+v", tuning)
	}
	if tuning.IndexPromotion {
		t.Fatalf("expected index promotion disabled, got %+v", tuning)
	}
}

func TestHTTPRangeReaderDedupesConcurrentFetch(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	var reqCount atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqCount.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release

		rng := r.Header.Get("Range")
		if rng != "bytes=0-7" {
			t.Errorf("unexpected range header %q", rng)
		}
		w.Header().Set("Content-Range", "bytes 0-7/26")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[:8])
	}))
	defer srv.Close()

	rd := NewHTTPRangeReader(srv.Client(), srv.URL, int64(len(data)), 8, 4, nil, nil)

	var wg sync.WaitGroup
	results := make([][]byte, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			buf := make([]byte, 4)
			_, err := rd.ReaderAt(context.Background()).ReadAt(buf, 0)
			results[i] = buf
			errs[i] = err
		}(i)
	}

	<-started
	time.Sleep(50 * time.Millisecond)
	if got := reqCount.Load(); got != 1 {
		t.Fatalf("expected one in-flight range request, got %d", got)
	}

	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil && err != io.EOF {
			t.Fatalf("read %d returned error: %v", i, err)
		}
		if !bytes.Equal(results[i], data[:4]) {
			t.Fatalf("read %d mismatch: got %q want %q", i, string(results[i]), string(data[:4]))
		}
	}
}

func TestHTTPRangeReaderRetainsMultipleRanges(t *testing.T) {
	data := bytes.Repeat([]byte("0123456789abcdef"), 8192)
	var reqs int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqs++
		start, end, ok := parseHTTPTestRangeHeader(r.Header.Get("Range"), int64(len(data)))
		if !ok {
			t.Fatalf("missing or invalid Range header: %q", r.Header.Get("Range"))
		}
		w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(data[start:end]); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	rd := NewHTTPRangeReader(
		srv.Client(),
		srv.URL,
		int64(len(data)),
		16,
		defaultTransportPageBytes,
		nil,
		nil,
	)
	reader := rd.ReaderAt(context.Background())

	buf := make([]byte, 4)
	for _, off := range []int64{0, 70000, 0} {
		n, err := reader.ReadAt(buf, off)
		if err != nil && err != io.EOF {
			t.Fatalf("ReadAt(%d) returned error: %v", off, err)
		}
		if n != 4 {
			t.Fatalf("expected 4 bytes from offset %d, got %d", off, n)
		}
	}
	if reqs != 2 {
		t.Fatalf("expected 2 HTTP requests for two distinct cached ranges, got %d", reqs)
	}
}

func TestHTTPRangeReaderFullResponseFallbackStats(t *testing.T) {
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	rd := NewHTTPRangeReader(srv.Client(), srv.URL, int64(len(data)), 4, 4, nil, nil)
	buf := make([]byte, 4)
	reader := rd.ReaderAt(context.Background())
	n, err := reader.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("first ReadAt returned error: %v", err)
	}
	if n != 4 || !bytes.Equal(buf, data[:4]) {
		t.Fatalf("first ReadAt returned n=%d data=%q, want %q", n, string(buf), string(data[:4]))
	}
	n, err = reader.ReadAt(buf, 8)
	if err != nil && err != io.EOF {
		t.Fatalf("second ReadAt returned error: %v", err)
	}
	if n != 4 || !bytes.Equal(buf, data[8:12]) {
		t.Fatalf("second ReadAt returned n=%d data=%q, want %q", n, string(buf), string(data[8:12]))
	}

	stats := rd.SnapshotStats()
	if stats.RangeRequestCount != 2 || stats.RangeResponseBytes != int64(len(data)) {
		t.Fatalf("unexpected range response stats: %+v", stats)
	}
	if stats.FullResponseFallbackCount != 1 {
		t.Fatalf("FullResponseFallbackCount = %d, want 1", stats.FullResponseFallbackCount)
	}
	if stats.FullResponseFallbackBytes != 4 || stats.LastFullResponseFallback != 4 {
		t.Fatalf("fallback bytes total=%d last=%d, want 4/4", stats.FullResponseFallbackBytes, stats.LastFullResponseFallback)
	}
}

func TestPackReaderRetriesIndexLoadAfterFailure(t *testing.T) {
	ctx := t.Context()
	packBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})

	type flakyTransport struct {
		data  []byte
		calls int
	}
	var ft flakyTransport
	ft.data = packBytes
	fetch := func(ctx context.Context, off int64, length int) ([]byte, error) {
		_ = ctx
		ft.calls++
		if ft.calls == 1 {
			return nil, errors.New("temporary trailer failure")
		}
		if off >= int64(len(ft.data)) {
			return nil, io.EOF
		}
		end := min(off+int64(length), int64(len(ft.data)))
		return bytes.Clone(ft.data[off:end]), nil
	}

	eng := NewPackReader("retry-pack", int64(len(packBytes)), TransportFunc(fetch), hash.HashType_HashType_SHA256)
	eng.SetExpectedBlockCount(1)
	eng.minWindow = 8
	eng.currentWindow = 8
	eng.maxWindow = 8

	keyHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := eng.getBlock(ctx, []byte(keyHash.MarshalString())); err == nil {
		t.Fatal("expected first read to fail during index load")
	}

	got, found, err := eng.getBlock(ctx, []byte(keyHash.MarshalString()))
	if err != nil {
		t.Fatalf("second read returned error: %v", err)
	}
	if !found || !bytes.Equal(got, []byte("alpha")) {
		t.Fatalf("expected retry to return alpha, found=%v data=%q", found, string(got))
	}
}

func TestBinarySearchEntriesByKeyUsesByteOrder(t *testing.T) {
	entries := []*kvfile.IndexEntry{
		{Key: []byte("11")},
		{Key: []byte("2")},
	}
	if got, found := binarySearchEntriesByKey(entries, []byte("2")); !found || string(got.GetKey()) != "2" {
		t.Fatalf("expected to find key 2, found=%v got=%v", found, got)
	}
	if got, found := binarySearchEntriesByKey(entries, []byte("11")); !found || string(got.GetKey()) != "11" {
		t.Fatalf("expected to find key 11, found=%v got=%v", found, got)
	}
}

func parseHTTPTestRangeHeader(h string, size int64) (start, end int64, ok bool) {
	var reqStart, reqEnd int64
	if _, err := fmt.Sscanf(h, "bytes=%d-%d", &reqStart, &reqEnd); err != nil {
		return 0, 0, false
	}
	if reqStart < 0 || reqEnd < reqStart || reqStart >= size {
		return 0, 0, false
	}
	if reqEnd >= size {
		reqEnd = size - 1
	}
	return reqStart, reqEnd + 1, true
}

// TransportFunc adapts a function to Transport for tests.
type TransportFunc func(ctx context.Context, off int64, length int) ([]byte, error)

func (f TransportFunc) Fetch(ctx context.Context, off int64, length int) ([]byte, error) {
	return f(ctx, off, length)
}
