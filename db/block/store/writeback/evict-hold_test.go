//go:build !js && !wasip1

package block_store_writeback

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// evictHoldStore measures queue-changing transactions on the native engine.
type evictHoldStore struct {
	kvtx.Store
	// holds contains the writer hold of each successful queue transaction.
	holds []time.Duration
	// entries counts the old entries deleted by successful transactions.
	entries int
}

// NewTransaction starts timing after the engine has acquired the writer.
func (s *evictHoldStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Leave reads alone and time writes after writer admission.
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil || !write {
		return tx, err
	}
	return &evictHoldTx{Tx: tx, store: s, start: time.Now()}, nil
}

// evictHoldTx tracks the queue changes made while holding the native writer.
type evictHoldTx struct {
	kvtx.Tx
	// store receives the completed transaction's measurement.
	store *evictHoldStore
	// start is recorded after NewTransaction acquired the writer.
	start time.Time
	// entries counts old eviction entries removed by this transaction.
	entries int
}

// Delete counts eviction entries while forwarding the real engine write.
func (t *evictHoldTx) Delete(ctx context.Context, key []byte) error {
	if bytes.HasPrefix(key, []byte(evictPrefix)) {
		t.entries++
	}
	return t.Tx.Delete(ctx, key)
}

// Commit measures through writer release; ordered eviction commits avoid fsync.
func (t *evictHoldTx) Commit(ctx context.Context) error {
	// Record only successful queue transactions, excluding the epoch advance.
	err := t.Tx.Commit(ctx)
	if err == nil && t.entries != 0 {
		t.store.holds = append(t.store.holds, time.Since(t.start))
		t.store.entries += t.entries
	}
	return err
}

// TestEvictRefusedWriterHold measures one 1,024-refusal pass per policy on
// an on-disk native Volume. Disabling optimistic transactions measures the
// whole writer hold, including the fallback used after repeated conflicts.
func TestEvictRefusedWriterHold(t *testing.T) {
	// Make the transaction body part of the measured writer hold in both arms.
	t.Setenv(kvtx.OptimisticEnv, "off")
	for _, retry := range []bool{false, true} {
		t.Run(fmt.Sprintf("retry=%t", retry), func(t *testing.T) {
			// Open the same s4db Volume used by native provider storage.
			SetEvictRefusedRetry(retry)
			t.Cleanup(func() { SetEvictRefusedRetry(false) })
			ctx := t.Context()
			vol, err := volume_s4db.NewVolume(ctx, logrus.NewEntry(logrus.New()), &volume_s4db.Config{
				Path: filepath.Join(t.TempDir(), "evict.s4wave"),
			})
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = vol.Close() })
			markers, release, err := vol.AccessObjectStore(ctx, testMarkersID, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			measured := &evictHoldStore{Store: markers}
			s, err := NewStore(ctx, ownedStore{vol}, vol, measured, testMarkersID, true)
			if err != nil {
				t.Fatal(err)
			}

			// Queue a full scan batch of unmarked local fills absent remotely.
			for i := range evictBatchBlocks {
				data := fmt.Appendf(nil, "writer-hold-refusal-%05d", i)
				ref, err := block.BuildBlockRef(data, nil)
				if err != nil {
					t.Fatal(err)
				}
				s.Refill(ref, &block.StoredBlock{Data: data, RefsKnown: true})
			}
			waitStatus(t, s, func(st Status) bool { return st.Refills == evictBatchBlocks })

			// Time only the actual queue transactions, excluding scan and probes.
			result, err := s.Evict(ctx, newInmemBlockStore(), 1<<30, nil, nil)
			if err != nil || result != (EvictResult{Refused: evictBatchBlocks}) || measured.entries != evictBatchBlocks {
				t.Fatalf("measured pass = %+v, %v, %d entries", result, err, measured.entries)
			}
			wantCommits := 1
			if retry {
				wantCommits = (evictBatchBlocks + evictRetryCommitBlocks - 1) / evictRetryCommitBlocks
			}
			if len(measured.holds) != wantCommits {
				t.Fatalf("queue transactions = %d, want %d", len(measured.holds), wantCommits)
			}
			var total, longest time.Duration
			for _, hold := range measured.holds {
				total += hold
				longest = max(longest, hold)
			}
			t.Logf("retry=%t entries=%d transactions=%d writer-total=%s writer-max=%s", retry, measured.entries, len(measured.holds), total, longest)
		})
	}
}
