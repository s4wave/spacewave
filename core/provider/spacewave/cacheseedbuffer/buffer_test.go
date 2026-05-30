package cacheseedbuffer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider/spacewave/seedreason"
)

// TestBufferOrderingAndEviction covers the ring-buffer invariants:
// oldest-first ordering, capacity-bounded storage, and eviction of the oldest
// entry when a new one arrives at capacity.
func TestBufferOrderingAndEviction(t *testing.T) {
	t.Run("snapshot_orders_oldest_first", func(t *testing.T) {
		buf := New(4)
		buf.Record(seedreason.ColdSeed, "/a")
		buf.Record(seedreason.GapRecovery, "/b")
		buf.Record(seedreason.Mutation, "/c")

		snap := buf.Snapshot()
		if len(snap) != 3 {
			t.Fatalf("snapshot len = %d, want 3", len(snap))
		}
		wantPaths := []string{"/a", "/b", "/c"}
		for i, want := range wantPaths {
			if snap[i].Path != want {
				t.Errorf("snapshot[%d].Path = %q, want %q", i, snap[i].Path, want)
			}
		}
		if snap[0].Reason != seedreason.ColdSeed {
			t.Errorf("snapshot[0].Reason = %q, want %q", snap[0].Reason, seedreason.ColdSeed)
		}
	})

	t.Run("eviction_evicts_oldest_at_capacity", func(t *testing.T) {
		buf := New(3)
		for i := range 5 {
			buf.Record(seedreason.ColdSeed, "/"+strconv.Itoa(i))
		}

		snap := buf.Snapshot()
		if len(snap) != 3 {
			t.Fatalf("snapshot len = %d, want 3", len(snap))
		}
		wantPaths := []string{"/2", "/3", "/4"}
		for i, want := range wantPaths {
			if snap[i].Path != want {
				t.Errorf("snapshot[%d].Path = %q, want %q", i, snap[i].Path, want)
			}
		}
	})

	t.Run("default_capacity_applied", func(t *testing.T) {
		buf := New(0)
		if got := buf.Capacity(); got != DefaultCapacity {
			t.Errorf("Capacity() = %d, want %d", got, DefaultCapacity)
		}
	})

	t.Run("timestamps_are_monotonic_nondecreasing", func(t *testing.T) {
		buf := New(8)
		for range 4 {
			buf.Record(seedreason.ColdSeed, "/p")
		}
		snap := buf.Snapshot()
		for i := 1; i < len(snap); i++ {
			if snap[i].TimestampMs < snap[i-1].TimestampMs {
				t.Errorf("timestamp regressed at index %d: %d < %d", i, snap[i].TimestampMs, snap[i-1].TimestampMs)
			}
		}
	})
}

// TestBufferSubscribe asserts that Subscribe returns a snapshot of existing
// entries plus a channel that receives future appends.
func TestBufferSubscribe(t *testing.T) {
	buf := New(8)
	buf.Record(seedreason.ColdSeed, "/seed-0")
	buf.Record(seedreason.ColdSeed, "/seed-1")

	snap, updates, release := buf.Subscribe()
	defer release()

	if len(snap) != 2 {
		t.Fatalf("initial snapshot len = %d, want 2", len(snap))
	}
	if snap[0].Path != "/seed-0" || snap[1].Path != "/seed-1" {
		t.Fatalf("initial snapshot paths = [%q, %q]", snap[0].Path, snap[1].Path)
	}

	buf.Record(seedreason.GapRecovery, "/live-0")
	buf.Record(seedreason.Mutation, "/live-1")

	deadline := time.After(2 * time.Second)
	got := make([]Entry, 0, 2)
	for len(got) < 2 {
		select {
		case entry := <-updates:
			got = append(got, entry)
		case <-deadline:
			t.Fatalf("timed out waiting for live updates; got %d", len(got))
		}
	}
	if got[0].Path != "/live-0" || got[0].Reason != seedreason.GapRecovery {
		t.Errorf("live[0] = %+v", got[0])
	}
	if got[1].Path != "/live-1" || got[1].Reason != seedreason.Mutation {
		t.Errorf("live[1] = %+v", got[1])
	}
}

func TestBufferSubscribeReleaseClosesUpdates(t *testing.T) {
	buf := New(2)
	_, updates, release := buf.Subscribe()
	release()

	select {
	case _, ok := <-updates:
		if ok {
			t.Fatal("updates channel remained open after release")
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for updates channel close")
	}
}

func TestBufferSubscribeUpdatesStartAfterSnapshot(t *testing.T) {
	buf := New(4)
	buf.Record(seedreason.ColdSeed, "/snapshot")

	snap, updates, release := buf.Subscribe()
	defer release()
	if len(snap) != 1 || snap[0].Path != "/snapshot" {
		t.Fatalf("snapshot = %+v, want /snapshot only", snap)
	}

	buf.Record(seedreason.Mutation, "/after")
	select {
	case entry := <-updates:
		if entry.Path != "/after" {
			t.Fatalf("update path = %q, want /after", entry.Path)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for post-snapshot update")
	}
}

func TestBufferSubscribeSlowConsumerDropsWithoutBlockingProducer(t *testing.T) {
	buf := New(1)
	_, updates, release := buf.Subscribe()
	defer release()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := range 64 {
			buf.Record(seedreason.ColdSeed, "/slow-"+strconv.Itoa(i))
		}
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Record blocked behind a slow subscriber")
	}

	waitForBufferedUpdate(t, updates)
	time.Sleep(100 * time.Millisecond)
	got := 0
	for {
		select {
		case <-updates:
			got++
		default:
			if got == 0 {
				t.Fatal("slow subscriber received no updates")
			}
			if got >= 64 {
				t.Fatalf("slow subscriber received all %d updates; expected drops", got)
			}
			return
		}
	}
}

func waitForBufferedUpdate(t *testing.T, updates <-chan Entry) {
	t.Helper()
	deadline := time.After(time.Second)
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		if len(updates) != 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for buffered update")
		case <-tick.C:
		}
	}
}

// TestBufferConcurrent records from multiple goroutines to exercise the mutex
// under -race and asserts the buffer never exceeds its capacity.
func TestBufferConcurrent(t *testing.T) {
	buf := New(32)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 64 {
				buf.Record(seedreason.ColdSeed, "/g"+strconv.Itoa(i)+"/"+strconv.Itoa(j))
			}
		}(i)
	}
	wg.Wait()

	snap := buf.Snapshot()
	if len(snap) > buf.Capacity() {
		t.Fatalf("snapshot len = %d, exceeds capacity %d", len(snap), buf.Capacity())
	}
	if len(snap) != buf.Capacity() {
		t.Fatalf("snapshot len = %d, want %d", len(snap), buf.Capacity())
	}
}

// TestRecordingTransport asserts the recording transport writes an entry for
// each request and still forwards the request to the wrapped base.
func TestRecordingTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	buf := New(8)
	cli := srv.Client()
	cli.Transport = NewRecordingTransport(cli.Transport, buf)

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/some/path", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(seedreason.Header, string(seedreason.ColdSeed))

	resp, err := cli.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()

	snap := buf.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snap))
	}
	if snap[0].Reason != seedreason.ColdSeed {
		t.Errorf("reason = %q, want %q", snap[0].Reason, seedreason.ColdSeed)
	}
	if snap[0].Path != "/some/path" {
		t.Errorf("path = %q, want %q", snap[0].Path, "/some/path")
	}
}
