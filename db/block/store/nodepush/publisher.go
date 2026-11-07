// Package block_store_nodepush sends each publication's new blocks from memory
// to a storage node, which holds them on a second disk. NodePush.tla specifies
// the protocol: a receipt waits for the node's flush as well as the local
// commit, and the node's absence never fails or stalls a commit.
package block_store_nodepush

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// Sender delivers blocks to the storage node.
type Sender interface {
	// Send returns nil once the node has flushed entries to its disk. It
	// returns promptly when ctx ends, and it reads entries no longer after it
	// returns: the publication's borrow of them ends then.
	Send(ctx context.Context, entries []*block.PutBatchEntry) error
}

// Result reports how one publication's push ended, after sent elapsed. A nil
// err means the node flushed it; otherwise the publication has only the local
// store's durability until it is uploaded.
type Result func(err error, sent time.Duration)

// Publisher wraps an AtomicPublisher so each receipt also waits for the node.
//
// Invariant: the receipt's result is the local store's result alone. The node
// adds a wait, bounded by max(wait, the local commit), and never an error, so
// a block the node lost is still queued for upload by the local store.
type Publisher struct {
	block.AtomicPublisher

	sender Sender
	wait   time.Duration
	result Result

	// on is the runtime switch; off, a publication is the local one.
	on *atomic.Bool
	// running joins the pushes of accepted publications.
	running sync.WaitGroup
}

// NewPublisher wraps inner, waiting at least wait for the node on each
// publication while on is set. result may be nil.
func NewPublisher(inner block.AtomicPublisher, sender Sender, wait time.Duration, on *atomic.Bool, result Result) *Publisher {
	return &Publisher{AtomicPublisher: inner, sender: sender, wait: wait, on: on, result: result}
}

// SubmitAtomic admits the publication to the local store and pushes its blocks
// beside it.
func (p *Publisher) SubmitAtomic(ctx context.Context, publication *block.AtomicPublication) (*block.PublicationReceipt, error) {
	// Order the local store by its own receipts: a later commit waits for the
	// earlier commit, not for the node's flush of it.
	local := *publication
	local.After = publication.After.Local()
	receipt, err := p.AtomicPublisher.SubmitAtomic(ctx, &local)
	if err != nil {
		return nil, err
	}

	// Push what the publication stores, if anything.
	entries := pushed(publication.Entries)
	if len(entries) == 0 || !p.on.Load() {
		return receipt, nil
	}
	outer := block.NewOuterPublicationReceipt(receipt)
	p.running.Add(1)
	go func() {
		defer p.running.Done()
		p.push(entries, receipt)
		outer.ResolveExisted(receipt.Err(), receipt.Existed(), receipt.Counting())
	}()
	return outer, nil
}

// PublishAtomic submits a publication and waits for its result with
// cancellation disabled after admission.
func (p *Publisher) PublishAtomic(ctx context.Context, publication *block.AtomicPublication) error {
	receipt, err := p.SubmitAtomic(ctx, publication)
	if err != nil {
		return err
	}
	defer receipt.Release()
	return receipt.Wait(context.WithoutCancel(ctx))
}

// Close joins the accepted pushes. The caller closes the inner publisher.
func (p *Publisher) Close() {
	p.running.Wait()
}

// push sends entries and returns once the send has ended and local is done.
// The node may use all of a slow local commit, which waits anyway, or wait if
// that is longer.
func (p *Publisher) push(entries []*block.PutBatchEntry, local *block.PublicationReceipt) {
	// End the send once the wait has passed and the local commit is done.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		timer := time.NewTimer(p.wait)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		select {
		case <-ctx.Done():
		case <-local.Done():
			cancel()
		}
	}()

	// Send, and hold the receipt until the local commit is done as well.
	started := time.Now()
	err := p.sender.Send(ctx, entries)
	if p.result != nil {
		p.result(err, time.Since(started))
	}
	<-local.Done()
}

// pushed returns the entries that store a block.
func pushed(entries []*block.PutBatchEntry) []*block.PutBatchEntry {
	out := make([]*block.PutBatchEntry, 0, len(entries))
	for _, entry := range entries {
		if entry != nil && !entry.Tombstone && !entry.Ref.GetEmpty() {
			out = append(out, entry)
		}
	}
	return out
}

// _ is a type assertion
var _ block.AtomicPublisher = (*Publisher)(nil)
