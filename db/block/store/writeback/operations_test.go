package block_store_writeback

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// gateStore holds each put until its gate is released, standing in for a
// slow write.
type gateStore struct {
	block.StoreOps
	// entered is sent once a put has begun.
	entered chan struct{}
	// gate is closed to let the puts finish.
	gate chan struct{}
}

// PutBlock waits on the gate, then writes.
func (g *gateStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	g.entered <- struct{}{}
	<-g.gate
	return g.StoreOps.PutBlock(ctx, data, opts)
}

// TestOpenOperations checks a write is open from before the store below
// writes until it returns, and a read operation from its start until its
// first release, each reported by its start.
func TestOpenOperations(t *testing.T) {
	ctx := t.Context()
	gate := &gateStore{StoreOps: newUntrackedBlockStore(), entered: make(chan struct{}, 1), gate: make(chan struct{})}
	m := NewMarkingStore(gate, func(context.Context, []Mark) error { return nil })
	if got := m.OldestOperations(); !got.Write.IsZero() || !got.Read.IsZero() {
		t.Fatalf("open operations on a new store: %v", got)
	}

	// A put is open while the store below holds it.
	before := time.Now()
	done := make(chan error, 1)
	go func() {
		_, _, err := m.PutBlock(ctx, []byte("slow"), leafOpts)
		done <- err
	}()
	<-gate.entered
	write := m.OldestOperations().Write
	if write.Before(before) || write.After(time.Now()) {
		t.Fatalf("put began at %v, not after %v", write, before)
	}

	// A read operation begun beside it is open until its release; a second
	// release changes nothing. The store below writes only once no read is
	// open, so the put waits for the release.
	_, release, err := m.BeginReadOperation(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := m.OldestOperations()
	if !got.Write.Equal(write) || got.Read.Before(write) {
		t.Fatalf("open operations beside the put: %v, put at %v", got, write)
	}
	release()
	release()
	if got := m.OldestOperations(); !got.Write.Equal(write) || !got.Read.IsZero() {
		t.Fatalf("open operations after the release: %v", got)
	}

	// Once the put returns, nothing is open.
	close(gate.gate)
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	if got := m.OldestOperations(); !got.Write.IsZero() || !got.Read.IsZero() {
		t.Fatalf("open operations after the put: %v", got)
	}
}

// TestOpenOperationsFlat checks recording a write allocates nothing, with few
// or many read operations open.
func TestOpenOperationsFlat(t *testing.T) {
	ctx := t.Context()
	m := NewMarkingStore(newUntrackedBlockStore(), nil)
	for _, open := range []int{10, 100} {
		var releases []func()
		for range open {
			_, release, err := m.BeginReadOperation(ctx)
			if err != nil {
				t.Fatal(err)
			}
			releases = append(releases, release)
		}
		if n := testing.AllocsPerRun(100, func() { m.ops.end(m.ops.begin(false)) }); n != 0 {
			t.Fatalf("a write with %d read operations open allocates %v", open, n)
		}
		for _, release := range releases {
			release()
		}
	}
}
