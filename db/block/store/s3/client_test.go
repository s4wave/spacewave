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
	"sync/atomic"
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

// TestDoReturnsPersistentFault fails every request with a 500 and checks
// that a GET returns the status after its bounded retries, rather than
// waiting like a throttle.
func TestDoReturnsPersistentFault(t *testing.T) {
	// Shorten the retry delays and fail every request.
	delays := retryDelays
	retryDelays = []time.Duration{time.Millisecond, time.Millisecond, time.Millisecond}
	t.Cleanup(func() { retryDelays = delays })
	var sent atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		http.Error(w, "<Error><Code>InternalError</Code></Error>", http.StatusInternalServerError)
	}))
	defer srv.Close()

	// The read fails with the status after one try and three retries.
	client := newTestClient(t, srv)
	_, err := client.GetObjectRange(t.Context(), "bucket", "key", 0, 8)
	var status *StatusError
	if !errors.As(err, &status) || status.StatusCode != http.StatusInternalServerError {
		t.Fatalf("err = %v; want the 500 status", err)
	}
	if got := sent.Load(); got != int32(len(retryDelays)+1) {
		t.Fatalf("sent %d requests; want %d", got, len(retryDelays)+1)
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

// TestInteractiveReadPassesBackgroundFlood floods a gate at its floor with
// background reads the bucket holds, and checks that an interactive read
// still completes at once: the flood holds only the capped lanes and leaves
// the reserve free.
func TestInteractiveReadPassesBackgroundFlood(t *testing.T) {
	// Hold every background request until the interactive read completes.
	bucket, client := newFakeBucket(t, map[string]string{"bg": "b", "fg": "f"})
	client.gate.limit = gateFloor
	client.gate.pool.SetLimit(gateFloor)
	hold := make(chan struct{})
	bucket.after = func(r *http.Request) {
		if r.Header.Get("X-Test") == "bg" {
			<-hold
		}
	}

	// Start the flood, wait for it to fill the capped lanes and queue, then
	// read interactively while it is held.
	const flood = 20
	done := make(chan error, flood)
	bg := block.WithBackground(t.Context())
	for range flood {
		go func() { done <- tagGet(client, bg, "bg", "bg") }()
	}
	waitQueued(t, client.gate, func(st GateState) bool {
		return st.InFlight == gateCappedFloor && st.QueuedBackground == flood-gateCappedFloor
	})
	if err := tagGet(client, t.Context(), "fg", "fg"); err != nil {
		t.Fatal(err)
	}

	// Release the flood; every read succeeds.
	close(hold)
	for range flood {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// TestBackgroundPutPassesForegroundLoad holds every lane of a gate at its
// floor with interactive reads, queues more of them and then a background
// PUT, and checks that the PUT is the next request the bucket sees once one
// read returns: a foreground load above the limit cannot starve uploads.
func TestBackgroundPutPassesForegroundLoad(t *testing.T) {
	// Hold each interactive read until released, and record arrivals.
	bucket, client := newFakeBucket(t, map[string]string{"fg": "f"})
	client.gate.limit = gateFloor
	client.gate.pool.SetLimit(gateFloor)
	release := make(chan struct{})
	arrivals := make(chan string, 64)
	bucket.after = func(r *http.Request) {
		arrivals <- r.Method
		if r.Method == http.MethodGet {
			<-release
		}
	}

	// Hold the limit, queue more reads, then the background PUT.
	const reads = gateFloor + 10
	done := make(chan error, reads+1)
	for range reads {
		go func() { done <- tagGet(client, t.Context(), "fg", "fg") }()
	}
	waitQueued(t, client.gate, func(st GateState) bool {
		return st.InFlight == gateFloor && st.QueuedInteractive == reads-gateFloor
	})
	for range gateFloor {
		<-arrivals
	}
	go func() {
		done <- client.PutObject(block.WithBackground(t.Context()), "bucket", "up", []byte("u"), "")
	}()
	waitQueued(t, client.gate, func(st GateState) bool { return st.QueuedBackground == 1 })

	// One read's return admits the PUT ahead of the queued reads.
	release <- struct{}{}
	if got := <-arrivals; got != http.MethodPut {
		t.Fatalf("next request %s; want the background PUT", got)
	}
	close(release)
	for range reads + 1 {
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// TestUrgentProbeEndsPause throttles one interactive read with a ten second
// Retry-After, lets the bucket recover, and checks that an urgent read
// completes after one probe interval rather than at the pause's deadline,
// and that its clean answer resumes the waiting interactive read too.
func TestUrgentProbeEndsPause(t *testing.T) {
	// Throttle the first request for ten seconds.
	bucket, client := newFakeBucket(t, map[string]string{"a": "1", "b": "2"})
	bucket.throttleNext = 1
	bucket.retryAfter = "10"

	// The interactive read is throttled and waits in the pause.
	start := time.Now()
	inter := make(chan error, 1)
	go func() { inter <- tagGet(client, t.Context(), "a", "inter") }()
	waitQueued(t, client.gate, func(st GateState) bool { return !st.PausedUntil.IsZero() && st.QueuedInteractive == 1 })

	// An urgent read probes the recovered bucket and ends the pause.
	urgent := block.WithReadClass(t.Context(), block.ReadUrgent)
	if err := tagGet(client, urgent, "b", "urgent"); err != nil {
		t.Fatal(err)
	}
	if err := <-inter; err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took > gateProbeEvery+time.Second {
		t.Fatalf("reads took %v; want about one probe interval of %v", took, gateProbeEvery)
	}
	if st, _ := client.gate.State(); !st.PausedUntil.IsZero() || st.Throttles != 1 {
		t.Fatalf("gate state %+v; want unpaused after one throttle", st)
	}
}

// tagGet sends one GET of key tagged for the fake bucket and closes its body.
func tagGet(client *Client, ctx context.Context, key, tag string) error {
	resp, err := client.do(ctx, http.MethodGet, "bucket", key, nil, nil, http.Header{"X-Test": {tag}})
	if err != nil {
		return err
	}
	return resp.Body.Close()
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
	client.gate = newRequestGate(srv.URL, "test")
	return client
}

// TestSharedFetchRisesAndOutlivesLeader starts a background fetch on a gate
// whose one lane is held, joins it with an urgent caller, and checks that
// the queued request moves to the urgent queue, that the leader's cancel
// leaves the fetch running, and that the follower gets the result.
func TestSharedFetchRisesAndOutlivesLeader(t *testing.T) {
	// Hold the gate's only lane with a held read.
	bucket, client := newFakeBucket(t, map[string]string{"x": "v", "h": "h"})
	client.gate.limit = 1
	client.gate.pool.SetLimit(1)
	hold := make(chan struct{})
	bucket.after = func(r *http.Request) {
		if r.Header.Get("X-Test") == "hold" {
			<-hold
		}
	}
	held := make(chan error, 1)
	go func() { held <- tagGet(client, t.Context(), "h", "hold") }()
	waitQueued(t, client.gate, func(st GateState) bool { return st.InFlight == 1 })

	// A background leader queues the shared fetch.
	var g fetchGroup[string]
	fetch := func(ctx context.Context) (string, error) {
		data, err := client.GetObjectRange(ctx, "bucket", "x", 0, 8)
		return string(data), err
	}
	leaderCtx, cancelLeader := context.WithCancel(block.WithBackground(t.Context()))
	leader := make(chan error, 1)
	go func() {
		_, err := g.do(leaderCtx, "x", fetch)
		leader <- err
	}()
	waitQueued(t, client.gate, func(st GateState) bool { return st.QueuedBackground == 1 })

	// An urgent follower raises the queued request to urgent.
	type result struct {
		val string
		err error
	}
	follower := make(chan result, 1)
	go func() {
		val, err := g.do(block.WithReadClass(t.Context(), block.ReadUrgent), "x", fetch)
		follower <- result{val, err}
	}()
	waitQueued(t, client.gate, func(st GateState) bool { return st.QueuedUrgent == 1 && st.QueuedBackground == 0 })

	// The leader leaves; the fetch still completes for the follower.
	cancelLeader()
	if err := <-leader; !errors.Is(err, context.Canceled) {
		t.Fatalf("leader err = %v; want its own cancel", err)
	}
	close(hold)
	if got := <-follower; got.err != nil || got.val != "v" {
		t.Fatalf("follower got %q, %v; want the object", got.val, got.err)
	}
	if err := <-held; err != nil {
		t.Fatal(err)
	}
}
