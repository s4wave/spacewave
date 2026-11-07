package block_store_nodepush

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// localStore admits publications and resolves their receipts when told to.
type localStore struct {
	block.AtomicPublisher
	receipts []*block.PublicationReceipt
	afters   []*block.PublicationReceipt
}

func (l *localStore) SubmitAtomic(_ context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	receipt := block.NewPublicationReceipt()
	l.receipts = append(l.receipts, receipt)
	l.afters = append(l.afters, p.After)
	return receipt, nil
}

// node answers each Send from flush, or waits for its context when flush is
// nil, and records when a Send was still running.
type node struct {
	flush   chan error
	sends   atomic.Int32
	running atomic.Int32
}

func (n *node) Send(ctx context.Context, _ []*block.PutBatchEntry) error {
	n.sends.Add(1)
	n.running.Add(1)
	defer n.running.Add(-1)
	if n.flush == nil {
		<-ctx.Done()
		return ctx.Err()
	}
	select {
	case err := <-n.flush:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func publication(t *testing.T, after *block.PublicationReceipt) *block.AtomicPublication {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte("x"), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &block.AtomicPublication{After: after, Entries: []*block.PutBatchEntry{{Data: []byte("x"), Ref: ref}}}
}

func resolved(r *block.PublicationReceipt) bool {
	select {
	case <-r.Done():
		return true
	default:
		return false
	}
}

func waitDone(t *testing.T, r *block.PublicationReceipt) {
	t.Helper()
	select {
	case <-r.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("receipt did not resolve")
	}
}

// TestPublisherReceipts walks a receipt through the node's answers: it waits
// for the flush, keeps the local result, gives up on a silent node without
// failing the commit, and never chains a later commit behind the node.
func TestPublisherReceipts(t *testing.T) {
	local := &localStore{}
	n := &node{flush: make(chan error, 1)}
	var results atomic.Int32
	p := NewPublisher(local, n, 20*time.Millisecond, func(error, time.Duration) { results.Add(1) })
	ctx := context.Background()

	// Off, the publication is the local one and nothing is sent.
	off, err := p.SubmitAtomic(ctx, publication(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if off != local.receipts[0] || n.sends.Load() != 0 {
		t.Fatal("a disabled publisher pushed")
	}
	p.SetEnabled(true)

	// The receipt waits for the node after the local commit.
	first, err := p.SubmitAtomic(ctx, publication(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	local.receipts[1].Resolve(nil)
	time.Sleep(5 * time.Millisecond)
	if resolved(first) {
		t.Fatal("receipt resolved before the node's flush")
	}

	// A later commit orders behind the local receipt, while the first still
	// waits for the node.
	second, err := p.SubmitAtomic(ctx, publication(t, first))
	if err != nil {
		t.Fatal(err)
	}
	if local.afters[2] != local.receipts[1] {
		t.Fatal("a later commit chained behind the node's flush")
	}
	n.flush <- nil
	waitDone(t, first)
	if first.Err() != nil {
		t.Fatal(first.Err())
	}

	// A node failure leaves the local result; a local failure is the result.
	n.flush <- errors.New("node away")
	failed := errors.New("local failed")
	local.receipts[2].Resolve(failed)
	waitDone(t, second)
	if !errors.Is(second.Err(), failed) {
		t.Fatalf("receipt result %v", second.Err())
	}

	// A silent node costs the wait and no error, and the send has ended by the
	// time the receipt returns the borrow of the entries.
	n.flush = nil
	silent, err := p.SubmitAtomic(ctx, publication(t, second))
	if err != nil {
		t.Fatal(err)
	}
	local.receipts[3].Resolve(nil)
	waitDone(t, silent)
	if silent.Err() != nil || n.running.Load() != 0 {
		t.Fatalf("silent node: err %v, sends running %d", silent.Err(), n.running.Load())
	}

	// A slow local commit gives the node all of its time past the wait.
	n.flush = make(chan error, 1)
	slow, err := p.SubmitAtomic(ctx, publication(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	n.flush <- nil
	local.receipts[4].Resolve(nil)
	waitDone(t, slow)
	p.Close()
	if results.Load() != 4 {
		t.Fatalf("results %d", results.Load())
	}
}
