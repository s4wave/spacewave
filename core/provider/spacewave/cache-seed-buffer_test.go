package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestCacheSeedBufferOrderingAndEviction covers the ring-buffer invariants:
// oldest-first ordering, capacity-bounded storage, and eviction of the oldest
// entry when a new one arrives at capacity.
func TestCacheSeedBufferOrderingAndEviction(t *testing.T) {
	t.Run("snapshot_orders_oldest_first", func(t *testing.T) {
		buf := NewCacheSeedBuffer(4)
		buf.Record(SeedReasonColdSeed, "/a")
		buf.Record(SeedReasonGapRecovery, "/b")
		buf.Record(SeedReasonMutation, "/c")

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
		if snap[0].Reason != SeedReasonColdSeed {
			t.Errorf("snapshot[0].Reason = %q, want %q", snap[0].Reason, SeedReasonColdSeed)
		}
	})

	t.Run("eviction_evicts_oldest_at_capacity", func(t *testing.T) {
		buf := NewCacheSeedBuffer(3)
		for i := range 5 {
			buf.Record(SeedReasonColdSeed, "/"+strconv.Itoa(i))
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
		buf := NewCacheSeedBuffer(0)
		if got := buf.Capacity(); got != DefaultCacheSeedBufferCapacity {
			t.Errorf("Capacity() = %d, want %d", got, DefaultCacheSeedBufferCapacity)
		}
	})

	t.Run("timestamps_are_monotonic_nondecreasing", func(t *testing.T) {
		buf := NewCacheSeedBuffer(8)
		for range 4 {
			buf.Record(SeedReasonColdSeed, "/p")
		}
		snap := buf.Snapshot()
		for i := 1; i < len(snap); i++ {
			if snap[i].TimestampMs < snap[i-1].TimestampMs {
				t.Errorf("timestamp regressed at index %d: %d < %d", i, snap[i].TimestampMs, snap[i-1].TimestampMs)
			}
		}
	})
}

// TestCacheSeedBufferSubscribe asserts that Subscribe returns a snapshot of
// existing entries plus a channel that receives future appends.
func TestCacheSeedBufferSubscribe(t *testing.T) {
	buf := NewCacheSeedBuffer(8)
	buf.Record(SeedReasonColdSeed, "/seed-0")
	buf.Record(SeedReasonColdSeed, "/seed-1")

	snap, updates, release := buf.Subscribe()
	defer release()

	if len(snap) != 2 {
		t.Fatalf("initial snapshot len = %d, want 2", len(snap))
	}
	if snap[0].Path != "/seed-0" || snap[1].Path != "/seed-1" {
		t.Fatalf("initial snapshot paths = [%q, %q]", snap[0].Path, snap[1].Path)
	}

	buf.Record(SeedReasonGapRecovery, "/live-0")
	buf.Record(SeedReasonMutation, "/live-1")

	deadline := time.After(2 * time.Second)
	got := make([]CacheSeedEntry, 0, 2)
	for len(got) < 2 {
		select {
		case entry := <-updates:
			got = append(got, entry)
		case <-deadline:
			t.Fatalf("timed out waiting for live updates; got %d", len(got))
		}
	}
	if got[0].Path != "/live-0" || got[0].Reason != SeedReasonGapRecovery {
		t.Errorf("live[0] = %+v", got[0])
	}
	if got[1].Path != "/live-1" || got[1].Reason != SeedReasonMutation {
		t.Errorf("live[1] = %+v", got[1])
	}
}

// TestCacheSeedBufferConcurrent records from multiple goroutines to exercise
// the mutex under -race and asserts the buffer never exceeds its capacity.
func TestCacheSeedBufferConcurrent(t *testing.T) {
	buf := NewCacheSeedBuffer(32)
	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			for j := range 64 {
				buf.Record(SeedReasonColdSeed, "/g"+strconv.Itoa(i)+"/"+strconv.Itoa(j))
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

// TestCacheSeedRecordingTransport asserts the recording transport writes an
// entry for each request and still forwards the request to the wrapped base.
func TestCacheSeedRecordingTransport(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	buf := NewCacheSeedBuffer(8)
	cli := &http.Client{Transport: NewCacheSeedRecordingTransport(nil, buf)}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL+"/some/path", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set(SeedReasonHeader, string(SeedReasonColdSeed))

	resp, err := cli.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	_ = resp.Body.Close()

	snap := buf.Snapshot()
	if len(snap) != 1 {
		t.Fatalf("snapshot len = %d, want 1", len(snap))
	}
	if snap[0].Reason != SeedReasonColdSeed {
		t.Errorf("reason = %q, want %q", snap[0].Reason, SeedReasonColdSeed)
	}
	if snap[0].Path != "/some/path" {
		t.Errorf("path = %q, want %q", snap[0].Path, "/some/path")
	}
}
