package block_store_s3

import (
	"context"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
)

// gatedStore holds each write transaction until the gate opens, and counts
// those opened after closed is set.
type gatedStore struct {
	kvtx.Store
	// gate releases the held writes when closed.
	gate chan struct{}
	// writes counts the write transactions opened.
	writes atomic.Int32
	// closed marks the index store as closed.
	closed atomic.Bool
	// late counts the transactions opened after closed.
	late atomic.Int32
}

// NewTransaction holds a write until the gate opens.
func (g *gatedStore) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if g.closed.Load() {
		g.late.Add(1)
	}
	if write {
		g.writes.Add(1)
		<-g.gate
	}
	return g.Store.NewTransaction(ctx, write)
}

// TestCloseTailsWaitsForDrain checks closing the tails waits for the step
// writing, drops the rest, and opens no transaction after it returns, so the
// index store may close after the pack store.
func TestCloseTailsWaitsForDrain(t *testing.T) {
	// Queue more tails than a step takes, and hold the first step's write.
	store := &gatedStore{Store: store_kvtx_inmem.NewStore(), gate: make(chan struct{})}
	x := newPackIndex(store)
	for i := range 3 * tailBatchRows {
		x.queueTail(fmt.Sprintf("pack-%d", i), []byte("tail"))
	}
	deadline := time.Now().Add(5 * time.Second)
	for store.writes.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("drain wrote nothing")
		}
		time.Sleep(time.Millisecond)
	}

	// Close waits for the held step.
	done := make(chan struct{})
	go func() {
		x.closeTails()
		close(done)
	}()
	select {
	case <-done:
		t.Fatal("closeTails returned with a step writing")
	case <-time.After(50 * time.Millisecond):
	}
	close(store.gate)
	<-done

	// Nothing writes after, and a new tail is refused.
	store.closed.Store(true)
	x.queueTail("pack-late", []byte("tail"))
	time.Sleep(50 * time.Millisecond)
	if n := store.late.Load(); n != 0 || store.writes.Load() != 1 {
		t.Fatalf("%d transactions after close, %d writes", n, store.writes.Load())
	}
	x.closeTails()
}
