//go:build !js && !wasip1

package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"runtime"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	store_kvtx_bolt "github.com/s4wave/spacewave/db/store/kvtx/bolt"
	"github.com/sirupsen/logrus"
)

// TestSyncDrainScaling measures a real durable queue, packer, and HTTP upload.
func TestSyncDrainScaling(t *testing.T) {
	for _, count := range []int{1000, 10000, 100000} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			// Persist metadata in the same native backend used by local volumes.
			ctx := t.Context()
			backend, err := store_kvtx_bolt.Open(filepath.Join(t.TempDir(), "queue.db"), 0600, nil, []byte("metadata"))
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := backend.Close(); err != nil {
					t.Error(err)
				}
			})
			metadata := &syncMeasuredStore{store: backend}
			if err := kvtx.RunTransaction(ctx, true, func(ctx context.Context) (kvtx.Tx, error) {
				return backend.NewTransaction(ctx, true)
			}, func(ctx context.Context, tx kvtx.Tx) error {
				return tx.Set(ctx, []byte("fixture"), []byte("sync"))
			}); err != nil {
				t.Fatal(err)
			}
			catalog, err := manifest.New(ctx, metadata)
			if err != nil {
				t.Fatal(err)
			}

			// Count actual upload requests and bytes with the production client.
			var requests, uploaded atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				n, err := io.Copy(io.Discard, r.Body)
				if err != nil {
					t.Error(err)
				}
				requests.Add(1)
				uploaded.Add(n)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(server.Close)
			key, peer := generateTestKeypair(t)
			client := NewSessionClient(server.Client(), server.URL, DefaultSigningEnvPrefix, key, peer.String())
			client.executeWriteTicketAudience = func(_ context.Context, _ string, _ writeTicketAudience, submit func(string) error) error {
				return submit("local-test-ticket")
			}
			logger := logrus.New()
			logger.SetOutput(io.Discard)
			syncer := &syncController{
				le: logrus.NewEntry(logger), store: metadata, client: client, resourceID: "scaling",
				mfst: catalog, lower: packfile_store.NewPackfileStore(nil, nil), upper: newSyncTestBlockStore(),
			}

			// Use unique small blocks so the block-count pack ceiling controls batching.
			marks := make([]block_store_writeback.Mark, 0, count)
			for n := range count {
				data := []byte("offline-block-" + strconv.Itoa(n))
				ref, _, err := syncer.upper.PutBlock(ctx, data, nil)
				if err != nil {
					t.Fatal(err)
				}
				marks = append(marks, block_store_writeback.Mark{Hash: ref.GetHash(), Size: int64(len(data))})
			}
			if err := syncer.MarkDirty(ctx, marks); err != nil {
				t.Fatal(err)
			}

			// Measure only draining, leaving setup outside allocation and visit totals.
			metadata.visits.Store(0)
			var before, after runtime.MemStats
			runtime.ReadMemStats(&before)
			started := time.Now()
			if err := syncer.FlushNowUnordered(ctx); err != nil {
				t.Fatal(err)
			}
			elapsed := time.Since(started)
			runtime.ReadMemStats(&after)
			if first, size, _ := syncer.pendingSnapshot(); !first.IsZero() || size != 0 {
				t.Fatalf("drained queue remains pending: first=%v bytes=%d", first, size)
			}
			if metadata.visits.Load() != int64(count*2) || metadata.page.Load() > syncDirtyPageLimit {
				t.Fatalf("unbounded queue work: visits=%d page=%d", metadata.visits.Load(), metadata.page.Load())
			}
			if requests.Load() != int64((count+4095)/4096) {
				t.Fatalf("paging changed pack count: %d", requests.Load())
			}
			t.Logf("blocks=%d visits=%d max-read=%d allocated=%d uploads=%d upload-bytes=%d elapsed=%s", count, metadata.visits.Load(), metadata.page.Load(), after.TotalAlloc-before.TotalAlloc, requests.Load(), uploaded.Load(), elapsed)
		})
	}
}
