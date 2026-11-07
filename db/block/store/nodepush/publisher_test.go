package block_store_nodepush

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// localStore admits publications and resolves their receipts when told to.
type localStore struct {
	block.AtomicPublisher
	receipts []*block.PublicationReceipt
	afters   []*block.PublicationReceipt
	admitted []*block.AtomicPublication
}

func (l *localStore) SubmitAtomic(_ context.Context, p *block.AtomicPublication) (*block.PublicationReceipt, error) {
	receipt := block.NewPublicationReceipt()
	l.receipts = append(l.receipts, receipt)
	l.afters = append(l.afters, p.After)
	l.admitted = append(l.admitted, p)
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
	return publicationOf(t, "x", after)
}

func publicationOf(t *testing.T, data string, after *block.PublicationReceipt) *block.AtomicPublication {
	t.Helper()
	ref, err := block.BuildBlockRef([]byte(data), nil)
	if err != nil {
		t.Fatal(err)
	}
	return &block.AtomicPublication{After: after, Entries: []*block.PutBatchEntry{{Data: []byte(data), Ref: ref, RefsKnown: true}}}
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
	var on atomic.Bool
	p := NewPublisher(local, n, 20*time.Millisecond, &on, func(error, time.Duration) { results.Add(1) })
	ctx := context.Background()

	// Off, the publication is the local one and nothing is sent.
	off, err := p.SubmitAtomic(ctx, publication(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if off != local.receipts[0] || n.sends.Load() != 0 {
		t.Fatal("a disabled publisher pushed")
	}
	on.Store(true)

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
	second, err := p.SubmitAtomic(ctx, publicationOf(t, "y", first))
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

	// Receipts resolve in commit order, whatever the node does: the second
	// waited for the first above, though its local commit failed first.

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

	// After a timeout commits stop waiting for the node for a while, and a
	// publication with unknown refs is never pushed.
	sends := n.sends.Load()
	quiet, err := p.SubmitAtomic(ctx, publication(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if n.sends.Load() != sends || quiet != local.receipts[4] {
		t.Fatal("a commit waited for a node that just timed out")
	}
	local.receipts[4].Resolve(nil)
	p.quietUntil.Store(0)
	unknown := publication(t, nil)
	unknown.Entries[0].RefsKnown = false
	plain, err := p.SubmitAtomic(ctx, unknown)
	if err != nil {
		t.Fatal(err)
	}
	if n.sends.Load() != sends || plain != local.receipts[5] {
		t.Fatal("pushed a block with unknown refs")
	}
	local.receipts[5].Resolve(nil)

	// A slow local commit gives the node all of its time past the wait.
	n.flush = make(chan error, 1)
	slow, err := p.SubmitAtomic(ctx, publication(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(60 * time.Millisecond)
	n.flush <- nil
	local.receipts[6].Resolve(nil)
	waitDone(t, slow)
	p.Close()
	if results.Load() != 5 {
		t.Fatalf("results %d", results.Load())
	}
}

// TestDurablePublisher walks the durable path: the commit is gated on the
// node's flush and names the block external, a failed push turns it back to
// a local entry before the gate opens, the memory set holds what the node
// flushed and is full at its limit, and a full memory set or a quiet node
// sends the publication down the other paths.
func TestDurablePublisher(t *testing.T) {
	local := &localStore{}
	n := &node{flush: make(chan error, 1)}
	var on, durable atomic.Bool
	on.Store(true)
	durable.Store(true)
	p := NewPublisher(local, n, time.Second, &on, nil)
	pub := publication(t, nil)
	value, err := block.EncodeBlockObject(pub.Entries[0].Data, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	var limit atomic.Int64
	limit.Store(int64(len(value)))
	held := NewHeld(&limit)
	p.SetDurable(&durable, held, func() bool { return true })
	var fallback atomic.Bool
	p.SetFallback(&fallback)
	ctx := context.Background()
	key := packfile.BlockKey(pub.Entries[0].Ref.GetHash())

	// The commit waits behind the gate until the node flushes.
	first, err := p.SubmitAtomic(ctx, pub)
	if err != nil {
		t.Fatal(err)
	}
	admitted := local.admitted[0]
	if !admitted.Entries[0].External || pub.Entries[0].External {
		t.Fatal("the durable commit names its block as stored, or changed the caller's entry")
	}
	if got, ok := held.Get(key); !ok || string(got) != string(value) {
		t.Fatal("the block is not readable from memory before its commit")
	}
	select {
	case <-admitted.Ready:
		t.Fatal("the commit's gate opened before the node's flush")
	case <-time.After(5 * time.Millisecond):
	}
	n.flush <- nil
	<-admitted.Ready
	if !admitted.Entries[0].External {
		t.Fatal("a flushed block lost its external mark")
	}

	// The memory set is full: the next publication takes the other path.
	second, err := p.SubmitAtomic(ctx, publicationOf(t, "y", first))
	if err != nil {
		t.Fatal(err)
	}
	if local.admitted[1].Ready != nil || local.admitted[1].Entries[0].External {
		t.Fatal("a publication past the memory limit took the durable path")
	}
	local.receipts[0].Resolve(nil)
	waitDone(t, first)
	if _, ok := held.Get(key); !ok {
		t.Fatal("the memory copy dropped at its commit, before the node's report")
	}
	n.flush <- nil
	local.receipts[1].Resolve(nil)
	waitDone(t, second)

	// The report's apply drops it. Without the fallback, a refused push is
	// sent again, and the gate opens only on the node's flush.
	held.Drop([][]byte{key})
	third, err := p.SubmitAtomic(ctx, publicationOf(t, "z", second))
	if err != nil {
		t.Fatal(err)
	}
	n.flush <- errors.New("refused")
	select {
	case <-local.admitted[2].Ready:
		t.Fatal("a refused push opened the gate without the fallback")
	case <-time.After(150 * time.Millisecond):
	}
	n.flush <- nil
	<-local.admitted[2].Ready
	if !local.admitted[2].Entries[0].External {
		t.Fatal("a block the node flushed on its second try lost its external mark")
	}
	local.receipts[2].Resolve(nil)
	waitDone(t, third)

	// With the fallback on, a failed push writes the block locally, and
	// releases the hold once its commit resolves.
	held.Drop([][]byte{packfile.BlockKey(local.admitted[2].Entries[0].Ref.GetHash())})
	fallback.Store(true)
	fourth, err := p.SubmitAtomic(ctx, publicationOf(t, "w", third))
	if err != nil {
		t.Fatal(err)
	}
	n.flush <- errors.New("refused")
	<-local.admitted[3].Ready
	if local.admitted[3].Entries[0].External {
		t.Fatal("a block the node refused stayed external")
	}
	local.receipts[3].Resolve(nil)
	waitDone(t, fourth)
	p.Close()
	if held.Bytes() != 0 {
		t.Fatal("a refused push left its block held")
	}
}
