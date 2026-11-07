// Package block_store_nodepush sends each root change's new blocks from memory
// to a storage node, which holds them on a second disk. NodePush.tla specifies
// the protocol: a root change waits for the node's flush as well as the local
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

// Publisher pushes each root change beside its local commit. The local result
// alone determines success; the node adds a wait bounded by the longer of wait
// and the local commit. Every send ends before the caller reuses its entries.
type Publisher struct {
	sender  Sender
	wait    time.Duration
	result  Result
	on      *atomic.Bool
	running sync.WaitGroup
}

// NewPublisher builds a publisher that pushes while on is set.
func NewPublisher(sender Sender, wait time.Duration, on *atomic.Bool, result Result) *Publisher {
	return &Publisher{sender: sender, wait: wait, on: on, result: result}
}

// SetRoots pushes entries beside commit, preserving its local result and roots.
func (p *Publisher) SetRoots(ctx context.Context, entries []*block.PutBatchEntry, roots []block.NamedRoot, commit func(context.Context, []*block.PutBatchEntry, []block.NamedRoot) error) error {
	// Commit without a push when disabled or when no block is stored.
	send := pushed(entries)
	if len(send) == 0 || !p.on.Load() {
		return commit(ctx, entries, roots)
	}

	// Keep the root change active until its borrowed entries are released.
	p.running.Add(1)
	defer p.running.Done()
	pushCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	defer cancel()

	// Bound the send by the longer of the local commit and the configured wait.
	localDone := make(chan struct{})
	sent := make(chan struct{})
	timer := time.NewTimer(p.wait)
	defer timer.Stop()
	go func() {
		select {
		case <-pushCtx.Done():
			return
		case <-timer.C:
		}
		select {
		case <-pushCtx.Done():
		case <-localDone:
			cancel()
		}
	}()

	// Send the block values alongside the local root change.
	go func() {
		defer close(sent)
		started := time.Now()
		err := p.sender.Send(pushCtx, send)
		if p.result != nil {
			p.result(err, time.Since(started))
		}
	}()

	// Finish the local change and join the send before returning borrowed entries.
	err := commit(ctx, entries, roots)
	close(localDone)
	<-sent
	return err
}

// Close joins the active root changes; the caller stops new changes first.
func (p *Publisher) Close() {
	p.running.Wait()
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
