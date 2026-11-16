package block_store_writeback

import (
	"context"
	"sync/atomic"

	"github.com/s4wave/spacewave/db/kvtx"
)

// evictAdmissionStore pauses one background request before writer admission.
// Foreground transactions still use the real engine without that pause.
type evictAdmissionStore struct {
	kvtx.Store
	// entered closes when a repair reaches writer admission.
	entered chan struct{}
	// release lets that background request proceed.
	release chan struct{}
	// paused selects only the first background writer.
	paused atomic.Bool
}

// NewTransaction exposes the writer-admission boundary without holding a writer.
func (s *evictAdmissionStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	// Pause only deferrable work, leaving a foreground disable free to commit.
	if write && kvtx.IsBackgroundWriteContext(ctx) && s.paused.CompareAndSwap(false, true) {
		close(s.entered)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-s.release:
		}
	}
	return s.Store.NewTransaction(ctx, write)
}

// _ verifies that the admission fixture preserves the store contract.
var _ kvtx.Store = (*evictAdmissionStore)(nil)
