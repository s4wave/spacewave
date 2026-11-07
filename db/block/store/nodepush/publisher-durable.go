package block_store_nodepush

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
)

// SetDurable turns on the durable path while durable and on are both set:
// pushed blocks skip the local disk and wait in held for the node's report.
// connected reports whether the node can take a push now. Call it before the
// first publication.
//
// The durable path is NodePush.tla's SubmitDurable, CommitDurable and
// PushFails. The publication's blocks go to held, readable at once, and to
// the node; the local store commits the publication, naming those blocks as
// external, without their bytes or a queue entry, only once the node flushed
// them, since the local write-ahead log survives a process crash and held does
// not. A push that fails or times out turns the entries back to local ones
// before the commit, which then writes them and their queue entries as the
// local path does. With the store journaled on the node, the receipt is
// durable once the node holds the commit's record too.
//
// Invariant: a local commit names a block as external only if the node
// flushed it, and held keeps it until the local index finds it on the remote
// or the local store holds it.
func (p *Publisher) SetDurable(durable *atomic.Bool, held *Held, connected func() bool) {
	p.durable, p.held, p.connected = durable, held, connected
}

// SetFallback sets the switch that lets a durable publication write its
// blocks locally when the node is away, quiet, or slower than the wait.
// Without it, or while it is off, the publication waits for the node's flush
// and its push tries again until the node takes it or the publisher closes.
// Call it before the first publication.
func (p *Publisher) SetFallback(fallback *atomic.Bool) {
	p.fallback = fallback
}

// falls reports whether a durable publication may write its blocks locally
// instead of waiting for the node.
func (p *Publisher) falls() bool {
	return p.fallback != nil && p.fallback.Load()
}

// submitDurable takes the publication on the durable path, reporting false
// when the path is off, the node is away or quiet, the publication pushes
// nothing, or held is full: the publication then takes the other paths.
func (p *Publisher) submitDurable(ctx context.Context, publication *block.AtomicPublication) (*block.PublicationReceipt, bool, error) {
	// Take only a publication the durable path can hold. Without the
	// fallback, a node that is away or quiet is waited for, not skipped.
	if p.durable == nil || p.held == nil || !p.durable.Load() || p.on == nil || !p.on.Load() {
		return nil, false, nil
	}
	if p.falls() && (!p.pushing() || (p.connected != nil && !p.connected())) {
		return nil, false, nil
	}
	pushes := pushed(publication.Entries)
	if len(pushes) == 0 {
		return nil, false, nil
	}

	// Mark the pushed entries external in a copy, and hold their values.
	local := *publication
	local.After = publication.After.Local()
	local.Entries = make([]*block.PutBatchEntry, len(publication.Entries))
	var external []*block.PutBatchEntry
	var keys, values [][]byte
	for i, entry := range publication.Entries {
		local.Entries[i] = entry
		if entry == nil || entry.Tombstone || entry.Ref.GetEmpty() || !entry.RefsKnown || entry.External {
			continue
		}
		value, err := block.EncodeBlockObject(entry.Data, entry.Refs, entry.RefsKnown)
		if err != nil {
			return nil, true, err
		}
		copied := *entry
		copied.External = true
		local.Entries[i] = &copied
		external = append(external, &copied)
		keys = append(keys, packfile.BlockKey(entry.Ref.GetHash()))
		values = append(values, value)
	}
	if !p.held.Add(keys, values) {
		return nil, false, nil
	}

	// Admit it, gated on the push.
	ready := make(chan struct{})
	local.Ready = ready
	receipt, err := p.AtomicPublisher.SubmitAtomic(ctx, &local)
	if err != nil {
		p.held.Remove(keys)
		return nil, true, err
	}

	// Push, then open the gate, and order the receipt behind the one before.
	previous := p.previousOuter()
	p.running.Add(1)
	go func() {
		defer p.running.Done()
		p.pushDurable(pushes, external, keys, ready, receipt)
	}()
	if previous == nil {
		p.setPrevious(receipt)
		return receipt, true, nil
	}
	return p.finishOuter(receipt, nil, previous), true, nil
}

// pushDurable sends entries and opens ready: as it is when the node flushed
// them, or with external turned back to local entries when the push failed
// or took longer than the wait. Their hold ends when the receipt resolves
// unless the node holds them.
func (p *Publisher) pushDurable(entries, external []*block.PutBatchEntry, keys [][]byte, ready chan<- struct{}, receipt *block.PublicationReceipt) {
	// Send until the node flushes them, or within the wait with the fallback.
	err := p.sendDurable(entries)

	// Without the node's flush the commit writes the blocks locally.
	if err != nil {
		for _, entry := range external {
			entry.External = false
		}
	}
	close(ready)

	// A local commit holds the blocks itself; a failed one names none.
	<-receipt.Done()
	if err != nil || receipt.Err() != nil {
		p.held.Remove(keys)
	}
}

// durableRetryMax bounds the pause between a durable push's tries.
const durableRetryMax = 2 * time.Second

// sendDurable sends entries until the node flushes them. With the fallback
// on, it tries once within the wait and quiets the node on a timeout; without
// it, a failed or late try is sent again, backing off to durableRetryMax,
// until the node takes it or the publisher closes.
func (p *Publisher) sendDurable(entries []*block.PutBatchEntry) error {
	pause := 100 * time.Millisecond
	for {
		// Send within the wait.
		ctx, cancel := context.WithTimeout(context.Background(), p.currentWait())
		started := time.Now()
		err := p.sender.Send(ctx, entries)
		late := err != nil && ctx.Err() != nil
		cancel()
		if p.result != nil {
			p.result(err, time.Since(started))
		}
		if err == nil {
			return nil
		}

		// Fall back, or wait for the node and send again.
		if p.falls() || p.closing.Load() {
			if late && p.falls() {
				p.quietUntil.Store(time.Now().Add(quietFor).UnixNano())
			}
			return err
		}
		time.Sleep(pause)
		pause = min(pause*2, durableRetryMax)
	}
}
