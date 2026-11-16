package provider_local

import (
	"context"
	"maps"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aperturerobotics/util/csync"
	"github.com/aperturerobotics/util/routine"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/sirupsen/logrus"
	logrustest "github.com/sirupsen/logrus/hooks/test"
)

// evictTestBackend fails the first remote confirmation, then delegates.
type evictTestBackend struct {
	backendStore
	// failed records whether the one transient failure has been returned.
	failed bool
}

// GetBlockExistsBatch returns a sentinel failure before confirming any block.
func (b *evictTestBackend) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Fail before any local value can be dropped.
	if !b.failed {
		b.failed = true
		return nil, block.ErrUnavailable
	}

	// Confirm only the actual backend's blocks on subsequent attempts.
	return b.backendStore.GetBlockExistsBatch(ctx, refs)
}

// TestEvictionRetriesTransientBackend uses the production routine backoff to
// retry a backend error without a restart and logs counters for the pass that
// evicted. Ported from Spacewave master c03394ac8, where it logs both passes;
// here the failed pass's counts are all zero, which writes no line.
func TestEvictionRetriesTransientBackend(t *testing.T) {
	// Upload through the real test backend and fail its first eviction probe.
	f := newFallbackTest(t)
	if _, _, err := f.wb.PutBlock(f.ctx, []byte("retry the backend probe"), &block.PutOpts{RefsKnown: true}); err != nil {
		t.Fatal(err)
	}
	f.upload()
	f.p.remote = new(atomic.Pointer[openBackend])
	f.p.remote.Store(&openBackend{store: &evictTestBackend{backendStore: f.packs}})
	f.p.evictGate = new(csync.Mutex)
	f.wb.SetPolicy(block_store_writeback.Policy{ResidentHigh: 1})
	logger, hook := logrustest.NewNullLogger()
	le := logrus.NewEntry(logger).WithFields(logrus.Fields{
		"bstore-id": "test-store", "account-id": "test-account", "routine": "storage-evict",
	})
	exits := make(chan error, 2)
	rc := routine.NewRoutineContainer(routine.WithRetry(providerBackoff), routine.WithExitCb(func(err error) { exits <- err }))
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	defer rc.ClearContext()
	rc.SetRoutine(func(ctx context.Context) error { return f.p.runEviction(ctx, le) })
	rc.SetContext(ctx, true)

	// The first pass returns the exact backend error, and its successor
	// drains the preserved queue through the routine's existing retry path.
	select {
	case err := <-exits:
		if !errors.Is(err, block.ErrUnavailable) {
			t.Fatalf("first pass error = %v, want unavailable", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("first pass never returned")
	}
	deadline := time.After(10 * time.Second)
	for {
		status, changed := f.wb.GetStatus()
		if status.EvictableBytes == 0 {
			break
		}
		select {
		case <-changed:
		case <-deadline:
			t.Fatal("transient backend error was not retried")
		}
	}

	// Join the routine before reading the completed-pass log record.
	cancel()
	select {
	case err := <-exits:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("routine exit = %v, want canceled", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("eviction routine never stopped")
	}
	entries := hook.AllEntries()
	if len(entries) != 1 {
		t.Fatalf("logged %d passes, want 1", len(entries))
	}
	for _, entry := range entries {
		if entry.Data["refused"] != 0 || entry.Data["requeued"] != 0 {
			t.Fatalf("pass counters = %v", entry.Data)
		}
		if entry.Message != "eviction pass" || len(entry.Data) != len(le.Data)+4 {
			t.Fatalf("pass log adds fields or text beyond counters: %+v", entry)
		}
		for key, value := range le.Data {
			if entry.Data[key] != value {
				t.Fatalf("pass log changed inherited field %s: %v", key, entry.Data)
			}
		}
		if _, ok := entry.Data["evicted"].(int); !ok {
			t.Fatalf("pass eviction count = %v", entry.Data)
		}
		if _, ok := entry.Data["evicted-bytes"].(int64); !ok {
			t.Fatalf("pass byte count = %v", entry.Data)
		}
	}
	if entries[0].Data["evicted"] != 1 {
		t.Fatalf("successful pass counters = %v", entries[0].Data)
	}
}

// TestEvictPassLogSums checks that the eviction loop sums its passes' counts
// and logs them at most every evictLogInterval, writes no line for counts that
// are all zero, and logs what is left unlogged when flushed on exit.
func TestEvictPassLogSums(t *testing.T) {
	logger, hook := logrustest.NewNullLogger()
	l := &evictPassLog{le: logrus.NewEntry(logger)}
	counts := func() []logrus.Fields {
		var out []logrus.Fields
		for _, entry := range hook.AllEntries() {
			out = append(out, entry.Data)
		}
		return out
	}
	start := time.Unix(1000, 0)
	pass := block_store_writeback.EvictResult{Evicted: 1, EvictedBytes: 10, Requeued: 2, Refused: 3}

	// A zero pass first writes nothing, and leaves the next pass logged at
	// once.
	l.add(block_store_writeback.EvictResult{}, start)
	if got := counts(); len(got) != 0 {
		t.Fatalf("zero pass logged %v", got)
	}

	// The first counted pass logs at once; passes within the interval sum.
	l.add(pass, start)
	l.add(pass, start.Add(time.Second))
	l.add(pass, start.Add(evictLogInterval-time.Nanosecond))
	l.due(start.Add(evictLogInterval - time.Nanosecond))
	if got := counts(); len(got) != 1 || got[0]["evicted"] != 1 || got[0]["evicted-bytes"] != int64(10) {
		t.Fatalf("first interval logged %v", got)
	}

	// The interval's end logs the two summed passes, from the ticker's due.
	l.due(start.Add(evictLogInterval))
	want := logrus.Fields{"evicted": 2, "evicted-bytes": int64(20), "requeued": 4, "refused": 6}
	if got := counts(); len(got) != 2 || !maps.Equal(got[1], want) {
		t.Fatalf("second log = %v, want %v", got, want)
	}

	// Nothing unlogged writes nothing; the exit flush logs a pass within
	// the interval.
	l.due(start.Add(3 * evictLogInterval))
	l.flush(start.Add(3 * evictLogInterval))
	if got := counts(); len(got) != 2 {
		t.Fatalf("idle loop logged %v", got[2:])
	}
	l.add(pass, start.Add(3*evictLogInterval+time.Second))
	if got := counts(); len(got) != 3 {
		t.Fatalf("pass after an idle interval logged %d lines, want 3", len(got))
	}
	l.add(pass, start.Add(3*evictLogInterval+2*time.Second))
	l.flush(start.Add(3*evictLogInterval + 3*time.Second))
	if got := counts(); len(got) != 4 || got[3]["refused"] != 3 {
		t.Fatalf("exit flush logged %v", got)
	}

	// Zero counts within an interval write no line at the interval's end.
	l.add(block_store_writeback.EvictResult{}, start.Add(4*evictLogInterval))
	l.due(start.Add(5 * evictLogInterval))
	l.flush(start.Add(5 * evictLogInterval))
	if got := counts(); len(got) != 4 {
		t.Fatalf("zero interval logged %v", got[4:])
	}
}

// TestEvictRefusedRetryOn checks that only the exact value "on" of
// EvictRefusedRetryEnv enables refused-entry retries.
func TestEvictRefusedRetryOn(t *testing.T) {
	for value, want := range map[string]bool{
		"on": true, "": false, "ON": false, "On": false, "1": false, "true": false,
		" on": false, "on ": false, "off": false, "yes": false,
	} {
		if got := evictRefusedRetryOn(value); got != want {
			t.Errorf("evictRefusedRetryOn(%q) = %v, want %v", value, got, want)
		}
	}
}
