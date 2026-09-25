package provider_spacewave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestDirtyTrackingRetriesPersistedBlocks repairs a marker failure after payload storage.
func TestDirtyTrackingRetriesPersistedBlocks(t *testing.T) {
	for _, batch := range []bool{false, true} {
		name := "single"
		if batch {
			name = "batch"
		}
		t.Run(name, func(t *testing.T) {
			ctx := t.Context()
			metadata := newSyncTestKvStore()
			syncer := &syncController{store: metadata}
			payload := []byte("accepted local bytes")
			ref, err := block.BuildBlockRef(payload, nil)
			if err != nil {
				t.Fatal(err)
			}
			injected := errors.New("metadata unavailable")
			fail := true
			store := block_store_writeback.NewMarkingStore(newSyncTestBlockStore(), func(ctx context.Context, h *hash.Hash, size int64) error {
				if fail {
					return injected
				}
				return syncer.MarkDirty(ctx, h, size)
			})
			put := func() error {
				if batch {
					return store.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: ref, Data: payload}})
				}
				_, _, err := store.PutBlock(ctx, payload, &block.PutOpts{ForceBlockRef: ref})
				return err
			}

			// Returning success before this marker would lose the upload on restart.
			if err := put(); !errors.Is(err, injected) {
				t.Fatalf("failed marker was acknowledged: %v", err)
			}
			fail = false
			if err := put(); err != nil {
				t.Fatal(err)
			}
			first, size, _ := syncer.pendingSnapshot()
			if first.IsZero() || size != int64(len(payload)) {
				t.Fatalf("missing recovered queue: first=%v size=%d", first, size)
			}

			// Concurrent identical retries retain one block and the original deadline.
			results := make(chan error, 24)
			for range 24 {
				go func() { results <- put() }()
			}
			for range 24 {
				if err := <-results; err != nil {
					t.Fatal(err)
				}
			}
			reopened := &syncController{store: metadata}
			if err := reopened.recalcDirtySize(ctx); err != nil {
				t.Fatal(err)
			}
			gotFirst, gotSize, _ := reopened.pendingSnapshot()
			if !gotFirst.Equal(first) || gotSize != size {
				t.Fatalf("reopened queue changed: first=%v size=%d", gotFirst, gotSize)
			}

			// Acknowledging the last block resets the deadline before the next write.
			candidates, err := reopened.scanDirtyCandidates(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err := reopened.cleanupDirtyCandidates(ctx, candidates); err != nil {
				t.Fatal(err)
			}
			if next, size, _ := reopened.pendingSnapshot(); !next.IsZero() || size != 0 {
				t.Fatal("acknowledged queue retained its deadline")
			}
			if err := reopened.MarkDirty(ctx, ref.GetHash(), int64(len(payload))); err != nil {
				t.Fatal(err)
			}
			if next, _, _ := reopened.pendingSnapshot(); !next.After(first) {
				t.Fatal("new queue inherited the previous deadline")
			}
		})
	}
}

// TestSyncDeadlineSurvivesContinuousWrites exercises the real pack and HTTP path.
func TestSyncDeadlineSurvivesContinuousWrites(t *testing.T) {
	uploaded := make(chan time.Time, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		select {
		case uploaded <- time.Now():
		default:
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	key, pid := generateTestKeypair(t)
	client := NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, pid.String())
	client.executeWriteTicketAudience = func(_ context.Context, _ string, _ writeTicketAudience, submit func(string) error) error {
		return submit("test-ticket")
	}
	syncer := newDirtySyncExecuteTestController(t, client, nil)
	syncer.conf.SizeThresholdBytes = 0
	candidates, err := syncer.scanDirtyCandidates(t.Context())
	if err != nil || len(candidates) != 1 {
		t.Fatalf("initial queue: %v (%d blocks)", err, len(candidates))
	}
	first, _, _ := syncer.pendingSnapshot()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- syncer.Execute(ctx) }()
	defer func() {
		cancel()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()

	// The configured one-second interval remains fixed while writes keep arriving.
	writes := time.NewTicker(50 * time.Millisecond)
	defer writes.Stop()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case <-writes.C:
			if err := syncer.MarkDirty(ctx, candidates[0].hash, candidates[0].size); err != nil {
				t.Fatal(err)
			}
		case at := <-uploaded:
			if at.Before(first.Add(time.Second)) {
				t.Fatal("ordinary upload bypassed its configured interval")
			}
			return
		case <-deadline.C:
			t.Fatal("continuous writes postponed the first-pending deadline")
		}
	}
}

// TestSyncMissingDirtyBlockPreservesPending rejects an incomplete local payload set.
func TestSyncMissingDirtyBlockPreservesPending(t *testing.T) {
	syncer := &syncController{
		le: logrus.NewEntry(logrus.New()), store: newSyncTestKvStore(),
		upper: newSyncTestBlockStore(), lower: packfile_store.NewPackfileStore(nil, nil),
	}
	ref, err := block.BuildBlockRef([]byte("missing payload"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := syncer.MarkDirty(t.Context(), ref.GetHash(), 15); err != nil {
		t.Fatal(err)
	}
	if err := syncer.FlushNowUnordered(t.Context()); !errors.Is(err, block.ErrNotFound) {
		t.Fatalf("missing payload was acknowledged: %v", err)
	}
	pending, err := syncer.scanDirtyCandidates(t.Context())
	if err != nil || len(pending) != 1 {
		t.Fatalf("failed upload lost its marker: %v (%d blocks)", err, len(pending))
	}
}
