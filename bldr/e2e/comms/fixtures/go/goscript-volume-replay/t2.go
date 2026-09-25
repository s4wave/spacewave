//go:build goscript

package goscript_volume_replay

import (
	"context"
	"syscall/js"

	block "github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume/workload"
)

// relayWorkerPath is the fixture's RPC relay worker script.
const relayWorkerPath = "/workers/rpc-relay.js"

// relay charges one worker round trip per engine call, as an engine in a
// device worker (T2) costs its caller. The engine itself runs locally.
type relay struct {
	*workerClient
}

// hop makes one round trip sending send bytes and receiving recv bytes.
func (r relay) hop(ctx context.Context, send, recv int) error {
	_, err := r.call(ctx, "hop", func(req js.Value) {
		req.Set("send", js.Global().Get("Uint8Array").New(send))
		req.Set("recv", recv)
	})
	return err
}

// openT2 opens the engine from open behind a relay, so every block,
// transaction, and journal call pays one device worker round trip. A
// transaction buffers its writes and sends them with its commit.
func openT2(open func(ctx context.Context) (engine, error)) func(ctx context.Context) (engine, error) {
	return func(ctx context.Context) (engine, error) {
		e, err := open(ctx)
		if err != nil {
			return engine{}, err
		}
		r := relay{workerClient: startWorker(relayWorkerPath)}
		reopen, destroy := e.reopen, e.destroy
		e.target = t2Target{Target: e.target, relay: r}
		e.reopen = func(ctx context.Context) (workload.Target, error) {
			t, err := reopen(ctx)
			if err != nil {
				return nil, err
			}
			return t2Target{Target: t, relay: r}, nil
		}
		e.destroy = func() error {
			r.terminate()
			return destroy()
		}
		return e, nil
	}
}

// t2Target is a replay target reached through a relay.
type t2Target struct {
	workload.Target
	// relay charges the round trips.
	relay relay
}

// NewTransaction opens a transaction in one round trip.
func (t t2Target) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	if err := t.relay.hop(ctx, 0, 0); err != nil {
		return nil, err
	}
	tx, err := t.Target.NewTransaction(ctx, write)
	if err != nil {
		return nil, err
	}
	return &t2Tx{Tx: tx, relay: t.relay}, nil
}

// BeginReadOperation opens a read scope in one round trip.
func (t t2Target) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	if err := t.relay.hop(ctx, 0, 0); err != nil {
		return nil, nil, err
	}
	ops, release, err := t.Target.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return t2Ops{StoreOps: ops, relay: t.relay}, release, nil
}

// PutBlock writes a block in one round trip.
func (t t2Target) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	return t2Ops{StoreOps: t.Target, relay: t.relay}.PutBlock(ctx, data, opts)
}

// PutBlockBatch writes a batch in one round trip.
func (t t2Target) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	return t2Ops{StoreOps: t.Target, relay: t.relay}.PutBlockBatch(ctx, entries)
}

// GetBlock reads a block in one round trip.
func (t t2Target) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	return t2Ops{StoreOps: t.Target, relay: t.relay}.GetBlock(ctx, ref)
}

// GetBlockExists checks a block in one round trip.
func (t t2Target) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	return t2Ops{StoreOps: t.Target, relay: t.relay}.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks blocks in one round trip.
func (t t2Target) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	return t2Ops{StoreOps: t.Target, relay: t.relay}.GetBlockExistsBatch(ctx, refs)
}

// StatBlock stats a block in one round trip.
func (t t2Target) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	return t2Ops{StoreOps: t.Target, relay: t.relay}.StatBlock(ctx, ref)
}

// RmBlock removes a block in one round trip.
func (t t2Target) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	return t2Ops{StoreOps: t.Target, relay: t.relay}.RmBlock(ctx, ref)
}

// Sync syncs in one round trip.
func (t t2Target) Sync(ctx context.Context) (bool, error) {
	return t2Ops{StoreOps: t.Target, relay: t.relay}.Sync(ctx)
}

// AppendJournal journals in one round trip.
func (t t2Target) AppendJournal(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	if err := t.relay.hop(ctx, edgeBytes(adds, removes), 0); err != nil {
		return err
	}
	return t.Target.AppendJournal(ctx, adds, removes)
}

// AppendJournalOrdered journals with write ordering in one round trip when
// the engine supports it.
func (t t2Target) AppendJournalOrdered(ctx context.Context, adds, removes []block_gc.RefEdge) error {
	oj, ok := t.Target.(workload.OrderedJournal)
	if !ok {
		return t.AppendJournal(ctx, adds, removes)
	}
	if err := t.relay.hop(ctx, edgeBytes(adds, removes), 0); err != nil {
		return err
	}
	return oj.AppendJournalOrdered(ctx, adds, removes)
}

// ReplayJournal replays the journal in one round trip.
func (t t2Target) ReplayJournal(ctx context.Context, apply func(adds, removes []block_gc.RefEdge) error) error {
	if err := t.relay.hop(ctx, 0, 0); err != nil {
		return err
	}
	return t.Target.ReplayJournal(ctx, apply)
}

// edgeBytes approximates the encoded size of journaled edges.
func edgeBytes(adds, removes []block_gc.RefEdge) int {
	var n int
	for _, edges := range [][]block_gc.RefEdge{adds, removes} {
		for _, e := range edges {
			n += len(e.Subject) + len(e.Object)
		}
	}
	return n
}

// t2Ops is block store operations reached through a relay.
type t2Ops struct {
	block.StoreOps
	// relay charges the round trips.
	relay relay
}

// PutBlock writes a block in one round trip.
func (o t2Ops) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	if err := o.relay.hop(ctx, len(data), 0); err != nil {
		return nil, false, err
	}
	return o.StoreOps.PutBlock(ctx, data, opts)
}

// PutBlockBatch writes a batch in one round trip.
func (o t2Ops) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) error {
	var n int
	for _, e := range entries {
		n += len(e.Data)
	}
	if err := o.relay.hop(ctx, n, 0); err != nil {
		return err
	}
	return o.StoreOps.PutBlockBatch(ctx, entries)
}

// GetBlock reads a block in one round trip.
func (o t2Ops) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := o.StoreOps.GetBlock(ctx, ref)
	if err != nil {
		return nil, false, err
	}
	return data, found, o.relay.hop(ctx, 0, len(data))
}

// GetBlockExists checks a block in one round trip.
func (o t2Ops) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if err := o.relay.hop(ctx, 0, 0); err != nil {
		return false, err
	}
	return o.StoreOps.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch checks blocks in one round trip.
func (o t2Ops) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	if err := o.relay.hop(ctx, 0, 0); err != nil {
		return nil, err
	}
	return o.StoreOps.GetBlockExistsBatch(ctx, refs)
}

// StatBlock stats a block in one round trip.
func (o t2Ops) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if err := o.relay.hop(ctx, 0, 0); err != nil {
		return nil, err
	}
	return o.StoreOps.StatBlock(ctx, ref)
}

// RmBlock removes a block in one round trip.
func (o t2Ops) RmBlock(ctx context.Context, ref *block.BlockRef) error {
	if err := o.relay.hop(ctx, 0, 0); err != nil {
		return err
	}
	return o.StoreOps.RmBlock(ctx, ref)
}

// Sync syncs in one round trip.
func (o t2Ops) Sync(ctx context.Context) (bool, error) {
	if err := o.relay.hop(ctx, 0, 0); err != nil {
		return false, err
	}
	return o.StoreOps.Sync(ctx)
}

// t2Tx is a transaction reached through a relay. Reads pay a round trip each;
// writes buffer and travel with the commit.
type t2Tx struct {
	kvtx.Tx
	// relay charges the round trips.
	relay relay
	// pending counts the buffered write bytes.
	pending int
}

// Get reads a key in one round trip.
func (t *t2Tx) Get(ctx context.Context, key []byte) ([]byte, bool, error) {
	data, found, err := t.Tx.Get(ctx, key)
	if err != nil {
		return nil, false, err
	}
	return data, found, t.relay.hop(ctx, len(key), len(data))
}

// Exists checks a key in one round trip.
func (t *t2Tx) Exists(ctx context.Context, key []byte) (bool, error) {
	if err := t.relay.hop(ctx, len(key), 0); err != nil {
		return false, err
	}
	return t.Tx.Exists(ctx, key)
}

// Set buffers a write.
func (t *t2Tx) Set(ctx context.Context, key, value []byte) error {
	t.pending += len(key) + len(value)
	return t.Tx.Set(ctx, key, value)
}

// Delete buffers a delete.
func (t *t2Tx) Delete(ctx context.Context, key []byte) error {
	t.pending += len(key)
	return t.Tx.Delete(ctx, key)
}

// Iterate opens an iterator in one round trip; its entries stream back.
func (t *t2Tx) Iterate(ctx context.Context, prefix []byte, sort, reverse bool) kvtx.Iterator {
	_ = t.relay.hop(ctx, len(prefix), 0)
	return t.Tx.Iterate(ctx, prefix, sort, reverse)
}

// Commit sends the buffered writes and commits in one round trip.
func (t *t2Tx) Commit(ctx context.Context) error {
	if err := t.relay.hop(ctx, t.pending, 0); err != nil {
		return err
	}
	return t.Tx.Commit(ctx)
}

// CommitOrdered sends the buffered writes and commits with write ordering in
// one round trip.
func (t *t2Tx) CommitOrdered(ctx context.Context) error {
	if err := t.relay.hop(ctx, t.pending, 0); err != nil {
		return err
	}
	return kvtx.CommitOrdered(ctx, t.Tx)
}
