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
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/net/hash"
)

// TestHTTPRangeReaderDefaults verifies default and explicit transport sizing.
func TestHTTPRangeReaderDefaults(t *testing.T) {
	// Verify the HTTP reader defaults for its budget and transport windows.
	rd := NewHTTPRangeReader(nil, "https://example.com/pack", 1024, 0, nil, nil)
	if limit := rd.budget.limit.Load(); limit != defaultResidentBudget {
		t.Fatalf("budget limit = %d, want %d", limit, defaultResidentBudget)
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

	// Verify an explicit HTTP window controls minimum, quantum, and current sizes.
	rd = NewHTTPRangeReader(nil, "https://example.com/pack", 1024, 16, nil, nil)
	if rd.minWindow != 16 {
		t.Fatalf("minWindow = %d, want 16", rd.minWindow)
	}
	if rd.transportQuantum != 16 {
		t.Fatalf("transportQuantum = %d, want 16", rd.transportQuantum)
	}
	if rd.currentWindow != 16 {
		t.Fatalf("currentWindow = %d, want 16", rd.currentWindow)
	}
}

// TestPackReaderPlanFetchLeftShiftsWithinGap preserves coverage near a gap end.
func TestPackReaderPlanFetchLeftShiftsWithinGap(t *testing.T) {
	// Configure a reader with a resident span beyond the requested gap.
	eng := NewPackReader("shift-pack", 8<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}))
	eng.minWindow = 1 << 20
	eng.transportQuantum = 1 << 20
	eng.maxWindow = 2 << 20
	eng.currentWindow = 2 << 20
	eng.sparseReads = false
	eng.spans = []*span{{off: 3 << 20, size: 1 << 20}}

	// Verify the fetch plan shifts left to cover the gap end.
	key := eng.planFetchLocked(5<<19, (5<<19)+1, 0)
	if key.off != 1<<20 || key.size != 2<<20 {
		t.Fatalf("planFetchLocked() = [%d,%d), want [%d,%d)", key.off, key.end(), int64(1<<20), int64(3<<20))
	}
}

// TestPackReaderSparsePlanCapsColdBackshift bounds speculative sparse reads.
func TestPackReaderSparsePlanCapsColdBackshift(t *testing.T) {
	// Configure sparse fetch sizing and a distant resident span.
	eng := NewPackReader("sparse-shift-pack", 10<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}))
	eng.minWindow = 256 << 10
	eng.transportQuantum = 256 << 10
	eng.maxWindow = 8 << 20
	eng.currentWindow = 8 << 20
	eng.sparseReads = true

	// Bound sparse cold windows and their locality distance.
	eng.sparseColdWindow = 256 << 10
	eng.sparseLocalityDistance = 512 << 10
	eng.spans = []*span{{off: 8 << 20, size: 256 << 10}}

	// Verify the cold sparse plan avoids speculative backshift.
	key := eng.planFetchLocked(5<<20, (5<<20)+1, 0)
	if key.off != 5<<20 || key.size != 256<<10 {
		t.Fatalf("sparse planFetchLocked() = [%d,%d), want [%d,%d)", key.off, key.end(), int64(5<<20), int64((5<<20)+(256<<10)))
	}
}

// TestPackReaderSparsePlanPromotesNearbyReads verifies locality grows read-ahead.
func TestPackReaderSparsePlanPromotesNearbyReads(t *testing.T) {
	// Configure a sparse reader with room to grow nearby fetches.
	eng := NewPackReader("sparse-local-pack", 10<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}))
	eng.minWindow = 256 << 10
	eng.transportQuantum = 256 << 10
	eng.maxWindow = 2 << 20
	eng.currentWindow = 2 << 20
	eng.sparseReads = true

	// Bound the cold sparse window and its promotion distance.
	eng.sparseColdWindow = 256 << 10
	eng.sparseLocalityDistance = 512 << 10

	// Verify the first sparse read uses the cold window.
	first := eng.planFetchLocked(1<<20, (1<<20)+1, 0)
	if first.size != 256<<10 {
		t.Fatalf("first sparse fetch size = %d, want %d", first.size, 256<<10)
	}

	// Verify a nearby sparse read promotes its fetch window.
	second := eng.planFetchLocked((1<<20)+(128<<10), (1<<20)+(128<<10)+1, 0)
	if second.size <= first.size {
		t.Fatalf("nearby sparse fetch size = %d, want promotion above %d", second.size, first.size)
	}
}

// TestPackReaderPlanFetchShrinksWhenCoveredOnBothSides prevents resident overlap.
func TestPackReaderPlanFetchShrinksWhenCoveredOnBothSides(t *testing.T) {
	// Configure a reader with resident spans on both sides of a gap.
	eng := NewPackReader("shrink-pack", 8<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}))
	eng.minWindow = 1 << 20
	eng.transportQuantum = 1 << 20
	eng.maxWindow = 2 << 20
	eng.currentWindow = 2 << 20
	eng.spans = []*span{
		{off: 0, size: 1 << 20},
		{off: 2 << 20, size: 1 << 20},
	}

	// Verify the fetch plan covers only the uncovered gap.
	key := eng.planFetchLocked(3<<19, (3<<19)+1, 0)
	if key.off != 1<<20 || key.size != 1<<20 {
		t.Fatalf("planFetchLocked() = [%d,%d), want [%d,%d)", key.off, key.end(), int64(1<<20), int64(2<<20))
	}
}

// TestPackReaderSnapshotStats verifies public cache and I/O accounting.
func TestPackReaderSnapshotStats(t *testing.T) {
	// Create a reader whose state supplies the statistics snapshot.
	eng := NewPackReader("stats-pack", 1024, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}))
	eng.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Seed resident spans and transport sizing under the reader lock.
		eng.currentWindow = 256
		eng.loading = map[fetchKey]*fetchLoad{
			{off: 0, size: 8}: {done: make(chan struct{})},
		}
		eng.spans = []*span{
			{off: 0, size: 8},
			{off: 8, size: 8},
		}
		eng.residentBytes = 16
		eng.published = map[string]struct{}{"a": {}, "b": {}}

		// Seed publication, verification, and transport counters.
		eng.writebackRunning = 1
		eng.verifyFailures = 1
		eng.writebackCount = 4
		eng.writebackErrors = 1
		eng.indexLoaded = true
		eng.recordFetchLocked(fetchKey{off: 0, size: 64}, 48)
	})

	// Verify the snapshot reports resident, transport, and writeback accounting.
	stats := eng.SnapshotStats()
	if stats.ResidentBytes != 16 || stats.SpanCount != 2 {
		t.Fatalf("unexpected resident stats: %+v", stats)
	}
	if stats.FetchCount != 1 || stats.FetchedBytes != 64 || stats.LastFetchBytes != 64 {
		t.Fatalf("unexpected fetch stats: %+v", stats)
	}
	if stats.RangeRequestCount != 1 || stats.RangeResponseBytes != 48 {
		t.Fatalf("unexpected range response stats: %+v", stats)
	}
	if stats.PublishedBlocks != 2 || stats.WritebackRunning != 1 || stats.VerifyFailures != 1 {
		t.Fatalf("unexpected writeback stats: %+v", stats)
	}
	if stats.WritebackCount != 4 || stats.WritebackErrors != 1 || !stats.IndexLoaded {
		t.Fatalf("unexpected publication stats: %+v", stats)
	}
}

// TestPackReaderTransportFetchMaxBytesClampsTuning preserves platform fetch caps.
func TestPackReaderTransportFetchMaxBytesClampsTuning(t *testing.T) {
	// Configure transport windows larger than the reader fetch cap.
	eng := NewPackReader("cap-pack", 16<<20, TransportFunc(func(context.Context, int64, int) ([]byte, error) {
		return nil, nil
	}))
	eng.setTransportFetchMaxBytes(2 << 20)
	eng.setTransportWindows(4<<20, 4<<20, 8<<20)

	// Verify transport tuning and planned fetches honor the cap.
	if eng.minWindow != 2<<20 {
		t.Fatalf("min window = %d, want %d", eng.minWindow, 2<<20)
	}
	if eng.transportQuantum != 2<<20 {
		t.Fatalf("transport quantum = %d, want %d", eng.transportQuantum, 2<<20)
	}
	if eng.maxWindow != 2<<20 {
		t.Fatalf("max window = %d, want %d", eng.maxWindow, 2<<20)
	}
	key := eng.planFetchLocked(0, 1, 0)
	if key.size > 2<<20 {
		t.Fatalf("planned fetch size = %d, want <= %d", key.size, 2<<20)
	}
}

// TestHTTPRangeReaderDedupesConcurrentFetch verifies readers share one HTTP request.
func TestHTTPRangeReaderDedupesConcurrentFetch(t *testing.T) {
	// Gate HTTP responses while counting concurrent range requests.
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	var reqCount atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})

	// Start an HTTP server serving the shared range response.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Count the range request and wait for the response gate.
		reqCount.Add(1)
		select {
		case started <- struct{}{}:
		default:
		}
		<-release

		// Verify the requested range and return its partial response.
		rng := r.Header.Get("Range")
		if rng != "bytes=0-7" {
			t.Errorf("unexpected range header %q", rng)
		}
		w.Header().Set("Content-Range", "bytes 0-7/26")
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[:8])
	}))
	defer srv.Close()

	// Create the HTTP reader shared by concurrent callers.
	rd := NewHTTPRangeReader(srv.Client(), srv.URL, int64(len(data)), 8, nil, nil)

	// Start two readers requesting the same initial bytes.
	var wg sync.WaitGroup
	results := make([][]byte, 2)
	errs := make([]error, 2)
	for i := range 2 {
		wg.Add(1)

		// Read the shared range and record the result for this caller.
		go func(i int) {
			// Read the shared HTTP bytes and record this caller's result.
			defer wg.Done()
			buf := make([]byte, 4)
			_, err := rd.ReaderAt(context.Background()).ReadAt(buf, 0)
			results[i] = buf
			errs[i] = err
		}(i)
	}

	// Verify concurrent reads share one in-flight HTTP request.
	<-started
	time.Sleep(50 * time.Millisecond)
	if got := reqCount.Load(); got != 1 {
		t.Fatalf("expected one in-flight range request, got %d", got)
	}

	// Release the HTTP response and await both readers.
	close(release)
	wg.Wait()

	// Verify both HTTP readers receive the expected bytes.
	for i, err := range errs {
		if err != nil && err != io.EOF {
			t.Fatalf("read %d returned error: %v", i, err)
		}
		if !bytes.Equal(results[i], data[:4]) {
			t.Fatalf("read %d mismatch: got %q want %q", i, string(results[i]), string(data[:4]))
		}
	}
}

// observedDoneContext signals when a reader starts observing cancellation.
type observedDoneContext struct {
	// Context supplies the underlying lifetime.
	context.Context
	// done closes on the first observation of Done.
	done chan struct{}
	// once serializes the observation signal across readers.
	once sync.Once
}

// Done exposes cancellation and records that the caller can now wait on it.
func (c *observedDoneContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.done) })
	return c.Context.Done()
}

// TestPackReaderCanceledLeaderDoesNotPoisonWaiter verifies shared transport survives caller cancellation.
func TestPackReaderCanceledLeaderDoesNotPoisonWaiter(t *testing.T) {
	// Create a transport held until cancellation or the response gate.
	data := []byte("abcdefgh")
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	eng := NewPackReader("cancel-leader", int64(len(data)), TransportFunc(func(ctx context.Context, off int64, length int) ([]byte, error) {
		// Notify the test when the shared transport first starts.
		if calls.Add(1) == 1 {
			close(started)
		}

		// Wait for reader shutdown or the shared transport response.
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-release:
			return bytes.Clone(data[off : off+int64(length)]), nil
		}
	}))
	t.Cleanup(eng.Close)
	eng.setTransportWindows(len(data), len(data), len(data))

	// Start the leading reader with an independently cancellable context.
	leaderCtx, cancelLeader := context.WithCancel(t.Context())
	leaderDone := make(chan error, 1)
	go func() {
		buf := make([]byte, len(data))
		_, err := eng.ReaderAt(leaderCtx).ReadAt(buf, 0)
		leaderDone <- err
	}()
	<-started

	// Start another reader and await its cancellation observation.
	waitCtx := &observedDoneContext{Context: t.Context(), done: make(chan struct{})}
	waiterDone := make(chan error, 1)
	var waiterData []byte
	go func() {
		waiterData = make([]byte, len(data))
		_, err := eng.ReaderAt(waitCtx).ReadAt(waiterData, 0)
		waiterDone <- err
	}()
	<-waitCtx.done

	// Verify canceling the leader leaves the shared waiter and transport live.
	cancelLeader()
	if err := <-leaderDone; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader error = %v, want context canceled", err)
	}
	select {
	case err := <-waiterDone:
		t.Fatalf("waiter returned before transport completed: %v", err)
	default:
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("transport calls after leader cancellation = %d, want one", got)
	}

	// Release the transport and verify the waiter receives the shared bytes.
	close(release)
	if err := <-waiterDone; err != nil {
		t.Fatalf("waiter error = %v", err)
	}
	if !bytes.Equal(waiterData, data) {
		t.Fatalf("waiter data = %q, want %q", waiterData, data)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("transport calls = %d, want one", got)
	}
}

// TestPackReaderCloseCancelsTransport verifies owner shutdown releases reads.
func TestPackReaderCloseCancelsTransport(t *testing.T) {
	// Hold the transport until the reader closes.
	started := make(chan struct{})
	transportDone := make(chan struct{})

	// Record transport startup and wait until reader shutdown.
	var calls atomic.Int32
	eng := NewPackReader("close", 8, TransportFunc(func(ctx context.Context, _ int64, _ int) ([]byte, error) {
		// Notify transport startup and completion around reader cancellation.
		calls.Add(1)
		close(started)
		<-ctx.Done()
		close(transportDone)
		return nil, ctx.Err()
	}))
	eng.setTransportWindows(8, 8, 8)

	// Close the reader during a read and expect the read to report it.
	readDone := make(chan error, 1)
	go func() {
		_, err := eng.ReaderAt(t.Context()).ReadAt(make([]byte, 8), 0)
		readDone <- err
	}()
	<-started
	eng.Close()
	<-transportDone

	// Verify reader shutdown cancels one transport request and releases the read.
	if err := <-readDone; !errors.Is(err, ErrPackReaderClosed) {
		t.Fatalf("read error = %v, want ErrPackReaderClosed", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("transport calls = %d, want one", got)
	}
}

// TestHTTPRangeReaderRetainsMultipleRanges verifies nonadjacent cache reuse.
func TestHTTPRangeReaderRetainsMultipleRanges(t *testing.T) {
	// Serve distinct HTTP ranges while counting requests.
	data := bytes.Repeat([]byte("0123456789abcdef"), 8192)
	var reqs int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse the requested range and reject invalid fixture requests.
		reqs++
		start, end, ok := parseHTTPTestRangeHeader(r.Header.Get("Range"), int64(len(data)))
		if !ok {
			t.Fatalf("missing or invalid Range header: %q", r.Header.Get("Range"))
		}

		// Return the requested partial HTTP response.
		w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
		w.WriteHeader(http.StatusPartialContent)
		if _, err := w.Write(data[start:end]); err != nil {
			t.Fatalf("write response: %v", err)
		}
	}))
	defer srv.Close()

	// Open a range reader with a small transport window.
	rd := NewHTTPRangeReader(
		srv.Client(),
		srv.URL,
		int64(len(data)),
		16,
		nil,
		nil,
	)
	reader := rd.ReaderAt(context.Background())

	// Read two distinct ranges and then revisit the first cached range.
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

	// Verify revisiting the cached range avoids another HTTP request.
	if reqs != 2 {
		t.Fatalf("expected 2 HTTP requests for two distinct cached ranges, got %d", reqs)
	}
}

// TestHTTPRangeReaderRetriesTransientFailure retries one 5xx response, retries
// a 429 response after its Retry-After delay, and never retries other client
// errors.
func TestHTTPRangeReaderRetriesTransientFailure(t *testing.T) {
	// Fail the first request of each case with status, then serve ranges.
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	var reqs atomic.Int32
	var status atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Fail the first request.
		if reqs.Add(1) == 1 {
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(int(status.Load()))
			return
		}

		// Serve the requested range.
		start, end, ok := parseHTTPTestRangeHeader(r.Header.Get("Range"), int64(len(data)))
		if !ok {
			t.Errorf("missing or invalid Range header: %q", r.Header.Get("Range"))
			return
		}
		w.Header().Set("Content-Length", strconv.FormatInt(end-start, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = w.Write(data[start:end])
	}))
	defer srv.Close()

	// A 503 is retried at once.
	status.Store(http.StatusServiceUnavailable)
	rd := NewHTTPRangeReader(srv.Client(), srv.URL, int64(len(data)), 4, nil, nil)
	buf := make([]byte, 4)
	n, err := rd.ReaderAt(context.Background()).ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("ReadAt after 503 returned error: %v", err)
	}
	if n != 4 || !bytes.Equal(buf, data[:4]) {
		t.Fatalf("ReadAt returned n=%d data=%q, want %q", n, buf, data[:4])
	}
	if got := reqs.Load(); got != 2 {
		t.Fatalf("requests = %d, want 2", got)
	}

	// A 429 is retried after its Retry-After delay.
	reqs.Store(0)
	status.Store(http.StatusTooManyRequests)
	rd = NewHTTPRangeReader(srv.Client(), srv.URL, int64(len(data)), 4, nil, nil)
	start := time.Now()
	if _, err := rd.ReaderAt(context.Background()).ReadAt(buf, 0); err != nil && err != io.EOF {
		t.Fatalf("ReadAt after 429 returned error: %v", err)
	}
	if waited := time.Since(start); waited < time.Second {
		t.Fatalf("retried after %v, want the 1s Retry-After", waited)
	}
	if got := reqs.Load(); got != 2 {
		t.Fatalf("requests after 429 = %d, want 2", got)
	}

	// A 404 is not retried.
	reqs.Store(0)
	status.Store(http.StatusNotFound)
	rd = NewHTTPRangeReader(srv.Client(), srv.URL, int64(len(data)), 4, nil, nil)
	if _, err := rd.ReaderAt(context.Background()).ReadAt(buf, 0); err == nil {
		t.Fatal("ReadAt after 404 succeeded, want error")
	}
	if got := reqs.Load(); got != 1 {
		t.Fatalf("requests after 404 = %d, want 1", got)
	}
}

// TestResidentBudgetEvictsAcrossReaders evicts the globally oldest span when
// readers share one budget.
func TestResidentBudgetEvictsAcrossReaders(t *testing.T) {
	// Share one resident budget across two byte-backed readers.
	ctx := context.Background()
	transport := TransportFunc(func(_ context.Context, _ int64, size int) ([]byte, error) {
		return make([]byte, size), nil
	})
	budget := newResidentBudget(300)
	a := NewPackReader("a", 1000, transport)
	b := NewPackReader("b", 1000, transport)
	a.setBudget(budget)
	b.setBudget(budget)

	// Populate both readers with enough spans to trigger global eviction.
	for _, r := range [][2]int64{{0, 100}, {500, 600}} {
		if _, err := a.fetchSpans(ctx, r[0], r[1], true); err != nil {
			t.Fatalf("a.fetchSpans(%d, %d): %v", r[0], r[1], err)
		}
	}
	if _, err := b.fetchSpans(ctx, 0, 200, true); err != nil {
		t.Fatalf("b.fetchSpans: %v", err)
	}

	// Verify global eviction removes the oldest span from the first reader.
	if used := budget.used.Load(); used != 300 {
		t.Fatalf("budget used = %d, want 300", used)
	}
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if len(a.spans) != 1 || a.spans[0].off != 500 {
			t.Errorf("a spans = %d, want only the span at 500", len(a.spans))
		}
	})
	b.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if len(b.spans) != 1 {
			t.Errorf("b spans = %d, want 1", len(b.spans))
		}
	})
}

// TestHTTPRangeReaderFullResponseFallbackStats accounts for servers that ignore Range.
func TestHTTPRangeReaderFullResponseFallbackStats(t *testing.T) {
	// Serve whole-pack responses for requests containing a Range header.
	data := []byte("abcdefghijklmnopqrstuvwxyz")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	}))
	defer srv.Close()

	// Read the first range from a server ignoring Range.
	rd := NewHTTPRangeReader(srv.Client(), srv.URL, int64(len(data)), 4, nil, nil)
	buf := make([]byte, 4)
	reader := rd.ReaderAt(context.Background())
	n, err := reader.ReadAt(buf, 0)
	if err != nil && err != io.EOF {
		t.Fatalf("first ReadAt returned error: %v", err)
	}
	if n != 4 || !bytes.Equal(buf, data[:4]) {
		t.Fatalf("first ReadAt returned n=%d data=%q, want %q", n, string(buf), string(data[:4]))
	}

	// Read another range and verify its payload after the whole response.
	n, err = reader.ReadAt(buf, 8)
	if err != nil && err != io.EOF {
		t.Fatalf("second ReadAt returned error: %v", err)
	}
	if n != 4 || !bytes.Equal(buf, data[8:12]) {
		t.Fatalf("second ReadAt returned n=%d data=%q, want %q", n, string(buf), string(data[8:12]))
	}

	// Verify statistics distinguish full-response fallback from range traffic.
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

// TestPackReaderRetriesIndexLoadAfterFailure verifies trailer errors do not poison the index.
func TestPackReaderRetriesIndexLoadAfterFailure(t *testing.T) {
	// Build the pack used for recovery after a failed index load.
	ctx := t.Context()
	packBytes, _ := buildTestPackOrdered(t, []struct{ Name, Data string }{{"a", "alpha"}})

	// Create a transport that fails only its first trailer request.
	type flakyTransport struct {
		data  []byte
		calls int
	}
	var ft flakyTransport
	ft.data = packBytes
	fetch := func(ctx context.Context, off int64, length int) ([]byte, error) {
		// Count transport calls and fail the first trailer fetch.
		_ = ctx
		ft.calls++
		if ft.calls == 1 {
			return nil, errors.New("temporary trailer failure")
		}

		// Bound later transport reads to the available pack bytes.
		if off >= int64(len(ft.data)) {
			return nil, io.EOF
		}
		end := min(off+int64(length), int64(len(ft.data)))
		return bytes.Clone(ft.data[off:end]), nil
	}

	// Configure a reader with small index-fetch windows.
	eng := NewPackReader("retry-pack", int64(len(packBytes)), TransportFunc(fetch))
	eng.SetExpectedBlockCount(1)
	eng.minWindow = 8
	eng.currentWindow = 8
	eng.maxWindow = 8

	// Verify the first target read reports the trailer failure.
	keyHash, err := hash.Sum(hash.HashType_HashType_SHA256, []byte("alpha"))
	if err != nil {
		t.Fatal(err)
	}
	key, ref := packfile.BlockKey(keyHash), block.NewBlockRef(keyHash)
	if _, err := eng.getBlock(ctx, key, ref); err == nil {
		t.Fatal("expected first read to fail during index load")
	}

	// Verify the next target read retries the index and returns alpha.
	got, err := eng.getBlock(ctx, key, ref)
	if err != nil {
		t.Fatalf("second read returned error: %v", err)
	}
	if !bytes.Equal(got.GetData(), []byte("alpha")) {
		t.Fatalf("expected retry to return alpha, got %q", string(got.GetData()))
	}
}

// TestBinarySearchEntriesByKeyUsesByteOrder preserves KVFile key ordering.
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

// parseHTTPTestRangeHeader bounds a valid single HTTP range to the fixture.
func parseHTTPTestRangeHeader(h string, size int64) (start, end int64, ok bool) {
	// Parse the fixture range and reject invalid or out-of-bounds starts.
	var reqStart, reqEnd int64
	if _, err := fmt.Sscanf(h, "bytes=%d-%d", &reqStart, &reqEnd); err != nil {
		return 0, 0, false
	}
	if reqStart < 0 || reqEnd < reqStart || reqStart >= size {
		return 0, 0, false
	}

	// Clamp the fixture range end to the available bytes.
	if reqEnd >= size {
		reqEnd = size - 1
	}
	return reqStart, reqEnd + 1, true
}
