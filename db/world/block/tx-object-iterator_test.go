package world_block

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/tx"
)

// newIteratorTestTx forks the engine head into a standalone read Tx.
func newIteratorTestTx(t *testing.T, ctx context.Context) *Tx {
	t.Helper()
	engine := newRetirementTestEngine(t, ctx)
	locked := engine.bcast.Lock()
	head := engine.head.readTx
	locked.Unlock()
	forked, err := head.Fork(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ftx := forked.(*Tx)
	t.Cleanup(ftx.Discard)
	return ftx
}

// TestTxIterateObjectsWithQueuedWriter checks IterateObjects does not deadlock
// when a writer queues on the Tx lock while the iterator is constructed.
func TestTxIterateObjectsWithQueuedWriter(t *testing.T) {
	ctx := t.Context()
	wtx := newIteratorTestTx(t, ctx)

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			unlock, err := wtx.rmtx.Lock(ctx, true)
			if err != nil {
				return
			}
			unlock()
		}
	}()
	defer func() {
		close(stop)
		<-done
	}()

	for range 20000 {
		iterCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		it := wtx.IterateObjects(iterCtx, "", false)
		err := it.Err()
		it.Close()
		cancel()
		if err != nil {
			t.Fatalf("IterateObjects: %v", err)
		}
	}
}

// TestTxObjectIteratorCloseAfterInitError checks Close on an iterator that
// failed to initialize does not panic.
func TestTxObjectIteratorCloseAfterInitError(t *testing.T) {
	ctx := t.Context()
	dtx := newIteratorTestTx(t, ctx)
	dtx.Discard()

	it := dtx.IterateObjects(ctx, "", false)
	if !errors.Is(it.Err(), tx.ErrDiscarded) {
		t.Fatalf("Err() = %v, want %v", it.Err(), tx.ErrDiscarded)
	}
	it.Close()
	if it.Valid() {
		t.Fatal("closed iterator is valid")
	}
}
