package provider_local

import (
	"context"
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
// retry a backend error without a restart and logs counters for both passes.
// Ported from Spacewave master c03394ac8.
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

	// Join the routine before reading the two completed-pass log records.
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
	if len(entries) != 2 {
		t.Fatalf("logged %d passes, want 2", len(entries))
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
	if entries[1].Data["evicted"] != 1 {
		t.Fatalf("successful pass counters = %v", entries[1].Data)
	}
}
