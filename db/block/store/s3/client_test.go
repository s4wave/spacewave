//go:build !tinygo

package block_store_s3

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestListObjectsFollowsContinuation lists a listing split across two pages
// and sends the first page's continuation token, including its reserved
// characters, with the second request.
func TestListObjectsFollowsContinuation(t *testing.T) {

	// Define the two listing pages keyed by continuation token.
	const token = "1ueGcxLPRx1Tr/XYExHnhbYLgveDs2J/wm36Hy4vbOwM="
	pages := map[string]string{
		"": `<ListBucketResult><IsTruncated>true</IsTruncated>` +
			`<Contents><Key>p/a&amp;x</Key><Size>10</Size></Contents>` +
			`<Contents><Key>p/b</Key><Size>20</Size></Contents>` +
			`<NextContinuationToken>` + token + `</NextContinuationToken></ListBucketResult>`,
		token: `<ListBucketResult><IsTruncated>false</IsTruncated>` +
			`<Contents><Key>p/c</Key><Size>5</Size></Contents></ListBucketResult>`,
	}

	// Serve the listing pages from a test server.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Validate the list request parameters.
		query := r.URL.Query()
		if r.URL.Path != "/bucket" || query.Get("list-type") != "2" || query.Get("prefix") != "p/" {
			http.Error(w, "unexpected request "+r.URL.String(), http.StatusBadRequest)
			return
		}

		// Serve the page matching the continuation token.
		page, ok := pages[query.Get("continuation-token")]
		if !ok {
			http.Error(w, "unknown token", http.StatusBadRequest)
			return
		}
		_, _ = io.WriteString(w, page)
	}))
	defer srv.Close()

	// List the objects through the client and collect keys and sizes.
	client := newTestClient(t, srv)
	var keys []string
	var size int64
	err := client.ListObjects(context.Background(), "bucket", "p/", func(key string, objectSize int64) error {
		keys = append(keys, key)
		size += objectSize
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	// Assert the listed keys and total size match the two pages.
	if !slices.Equal(keys, []string{"p/a&x", "p/b", "p/c"}) || size != 35 {
		t.Fatalf("listed %v with %d bytes; want [p/a&x p/b p/c] with 35 bytes", keys, size)
	}
}

// TestDoRetriesTransientStatus retries a PUT the service fails with a 500 and
// sends the same body again.
func TestDoRetriesTransientStatus(t *testing.T) {

	// Record each PUT body and fail the first request with a 500.
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(body))
		if len(bodies) == 1 {
			http.Error(w, "<Error><Code>InternalError</Code></Error>", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()

	// Put an object and assert the body was sent twice.
	client := newTestClient(t, srv)
	err := client.PutObject(context.Background(), "bucket", "key", []byte("data"), "")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(bodies, []string{"data", "data"}) {
		t.Fatalf("bodies = %q; want the body sent twice", bodies)
	}
}

// TestThrottleBurstPausesUrgentReads throttles a burst of urgent reads with a
// one second Retry-After and checks that every read still succeeds, that the
// gate halved its limit, and that the bucket saw no request during the pause.
func TestThrottleBurstPausesUrgentReads(t *testing.T) {
	// Hold sixteen objects and throttle the first sixteen requests.
	const n = 16
	objects := make(map[string]string, n)
	for i := range n {
		objects["k"+strconv.Itoa(i)] = "v" + strconv.Itoa(i)
	}
	bucket, client := newFakeBucket(t, objects)
	bucket.throttleNext = n
	bucket.retryAfter = "1"

	// Record when each request arrives after the throttled burst.
	var mtx sync.Mutex
	var firstThrottle time.Time
	var arrived, early int
	bucket.after = func(*http.Request) {
		mtx.Lock()
		defer mtx.Unlock()
		if arrived++; arrived == 1 {
			firstThrottle = time.Now()
		}
		if arrived > n && time.Since(firstThrottle) < 900*time.Millisecond {
			early++
		}
	}

	// Read every object at once as urgent.
	ctx := block.WithReadClass(t.Context(), block.ReadUrgent)
	errs := make(chan error, n)
	for i := range n {
		go func() {
			data, err := client.GetObjectRange(ctx, "bucket", "k"+strconv.Itoa(i), 0, 8)
			if err == nil && string(data) != "v"+strconv.Itoa(i) {
				err = errors.New("read " + string(data))
			}
			errs <- err
		}()
	}
	for range n {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}

	// The gate paused for the Retry-After and halved its limit, once for
	// each second the throttled answers spanned.
	mtx.Lock()
	defer mtx.Unlock()
	if early != 0 {
		t.Fatalf("%d requests reached the bucket during the pause", early)
	}
	st, _ := client.gate.State()
	if st.Throttles != n || st.Limit > gateStart/2 || !st.PausedUntil.IsZero() || st.InFlight != 0 {
		t.Fatalf("gate state %+v; want %d throttles, limit at most %d, unpaused", st, n, gateStart/2)
	}
}

// TestInteractiveReadPassesBackgroundFlood holds a gate of one lane with a
// background read, queues a flood of background reads and then one
// interactive read, and checks that the interactive read takes the lane next.
func TestInteractiveReadPassesBackgroundFlood(t *testing.T) {
	// Hold the first request until the others queue, and record the order of
	// the requests.
	bucket, client := newFakeBucket(t, map[string]string{"bg": "b", "fg": "f"})
	client.gate.limit = 1
	client.gate.pool.SetLimit(1)
	hold := make(chan struct{})
	var mtx sync.Mutex
	var order []string
	bucket.after = func(r *http.Request) {
		// Record the request, and hold the first.
		mtx.Lock()
		order = append(order, r.Header.Get("X-Test"))
		first := len(order) == 1
		mtx.Unlock()
		if first {
			<-hold
		}
	}

	// Send each read tagged, so the bucket can record its order.
	const flood = 20
	done := make(chan error, flood+1)
	get := func(ctx context.Context, key, tag string) {
		// Send one tagged GET.
		resp, err := client.do(ctx, http.MethodGet, "bucket", key, nil, nil, http.Header{"X-Test": {tag}})
		if err == nil {
			_ = resp.Body.Close()
		}
		done <- err
	}

	// Start the flood, then the interactive read once the flood queues, and
	// release the held read once the interactive read queues.
	bg := block.WithBackground(t.Context())
	for range flood {
		go get(bg, "bg", "bg")
	}
	waitQueued(t, client.gate, func(st GateState) bool { return st.QueuedCapped == flood-1 })
	go get(t.Context(), "fg", "fg")
	waitQueued(t, client.gate, func(st GateState) bool { return st.QueuedInteractive == 1 })
	close(hold)

	// Every read succeeds.
	for range flood + 1 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}

	// The interactive read took the released lane. The held read's clean
	// answer also raises the limit to two, so one background read may be
	// sent alongside it and arrive first.
	mtx.Lock()
	defer mtx.Unlock()
	if len(order) != flood+1 || slices.Index(order, "fg") > 2 {
		t.Fatalf("request order %v; want the interactive read next", order)
	}
}

// waitQueued waits until the gate's state satisfies ok.
func waitQueued(t *testing.T, g *RequestGate, ok func(GateState) bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st, _ := g.State()
		if ok(st) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("gate state %+v never matched", st)
		}
		// Queue lengths have no watch; the test polls them briefly.
		time.Sleep(5 * time.Millisecond)
	}
}

// TestCheckBucketDeletesStaleProbes leaves the probe of an interrupted check
// out of the usage and deletes it.
func TestCheckBucketDeletesStaleProbes(t *testing.T) {

	// Seed a fake bucket with a block and a stale probe object.
	objects := map[string]string{
		"p/block":                  "0123456789",
		"p/.spacewave-check/stale": "probe",
	}
	bucket, client := newFakeBucket(t, objects)

	// Run the bucket check against the fake bucket.
	result := CheckBucket(context.Background(), client, "bucket", "p/")
	if result.GetOutcome() != CheckOutcome_CHECK_OUTCOME_OK {
		t.Fatalf("check = %v: %s", result.GetOutcome(), result.GetDetail())
	}

	// Assert the usage excludes the stale probe and it was deleted.
	if result.GetUsage().GetObjects() != 1 || result.GetUsage().GetBytes() != 10 {
		t.Fatalf("usage = %v; want 1 object of 10 bytes", result.GetUsage())
	}
	if keys := bucket.keys(); !slices.Equal(keys, []string{"p/block"}) {
		t.Fatalf("objects = %v; want only p/block", keys)
	}
}

// newTestClient builds a client for the test server.
func newTestClient(t *testing.T, srv *httptest.Server) *Client {
	// Build the client with a gate of its own, so no test inherits another's
	// throttle through a reused port.
	t.Helper()
	client, err := BuildClient(&ClientConfig{
		Endpoint:    strings.TrimPrefix(srv.URL, "http://"),
		DisableSsl:  true,
		Credentials: &Credentials{AccessKeyId: "key", SecretAccessKey: "secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	client.gate = newRequestGate(srv.URL)
	return client
}
