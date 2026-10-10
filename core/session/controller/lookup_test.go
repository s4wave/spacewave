package session_controller

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/session"
	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// sizeBlockStore blocks every transaction's Size until unblock closes.
type sizeBlockStore struct {
	kvtx.Store
	// sizing receives once per blocked Size call.
	sizing chan struct{}
	// unblock releases the blocked Size calls when closed.
	unblock chan struct{}
}

func (s *sizeBlockStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	tx, err := s.Store.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &sizeBlockTx{Tx: tx, store: s}, nil
}

type sizeBlockTx struct {
	kvtx.Tx
	store *sizeBlockStore
}

func (t *sizeBlockTx) Size(ctx context.Context) (uint64, error) {
	t.store.sizing <- struct{}{}
	select {
	case <-t.store.unblock:
		return t.Tx.Size(ctx)
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

// TestGetSessionByIdxDuringBlockedList checks that a Session lookup completes
// while another caller's storage read is still pending.
func TestGetSessionByIdxDuringBlockedList(t *testing.T) {
	// Store one Session registration.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	inner := store_kvtx_inmem.NewStore()
	setup := &Controller{objStore: inner}
	entry, err := setup.RegisterSession(ctx, &session.SessionRef{}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Wrap the store so its Size read blocks.
	store := &sizeBlockStore{
		Store:   inner,
		sizing:  make(chan struct{}, 1),
		unblock: make(chan struct{}),
	}
	controller := &Controller{objStore: store}

	// Start a list that stalls inside its storage read.
	listed := make(chan error, 1)
	go func() {
		_, err := controller.ListSessions(ctx)
		listed <- err
	}()
	<-store.sizing

	// Look up the Session while the list is stalled.
	got, err := controller.GetSessionByIdx(ctx, entry.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	if got.GetSessionIndex() != entry.GetSessionIndex() {
		t.Fatalf("session index = %d, want %d", got.GetSessionIndex(), entry.GetSessionIndex())
	}

	// Release the list and require it to finish.
	close(store.unblock)
	if err := <-listed; err != nil {
		t.Fatal(err)
	}
}
