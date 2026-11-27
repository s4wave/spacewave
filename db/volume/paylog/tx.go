package paylog

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
)

// Tx is a transaction on a Domain. A write transaction holds the store's
// writer, so its reads see every published change and no other commit falls
// inside it. The blocks it puts and removes are its own until it ends.
// Like any kvtx.Tx, it and its block store take one call at a time.
type Tx struct {
	// Tx is the key-value view under the domain's prefix.
	kvtx.Tx
	// s is the store.
	s *Store
	// itx is the index transaction.
	itx kvtx.Tx
	// write is set on a write transaction.
	write bool
	// unlock releases the writer, nil once the transaction ended.
	unlock func()
	// own holds the transaction's unpublished block writes and removes by
	// block key, guarded by s.mtx.
	own map[string]*pendingBlock
	// blocks serves the transaction's block operations.
	blocks blockStore
}

// Priorities of Store.writer.
const (
	// writerBackground is the priority of write transactions opened with
	// block.WithBackground.
	writerBackground = iota - 1
	// writerForeground is the priority of every other write transaction.
	writerForeground
)

// begin opens a transaction whose key-value view keeps keys under prefix. A
// write transaction first waits for the writer under ctx, behind every waiting
// foreground writer when ctx carries block.WithBackground and the store does
// not admit its writers equally.
func (s *Store) begin(ctx context.Context, write bool, prefix []byte) (*Tx, error) {
	// Take the writer for a write transaction.
	t := &Tx{s: s, write: write, unlock: func() {}}
	if write {
		prio := writerForeground
		if block.IsBackground(ctx) && !s.equalWriters {
			prio = writerBackground
		}
		s.waiting.Add(1)
		unlock, err := s.writer.Lock(ctx, prio)
		s.waiting.Add(-1)
		if err != nil {
			return nil, err
		}
		t.unlock = unlock
		t.own = make(map[string]*pendingBlock)
	}

	// Open the index transaction under the writer.
	itx, err := s.index.NewTransaction(ctx, write)
	if err != nil {
		t.unlock()
		return nil, err
	}
	t.itx = itx
	t.Tx = kvtx_prefixer.NewTx(itx, prefix)
	t.blocks = blockStore{s: s, tx: t}
	return t, nil
}

// GetBlockStore returns the transaction's block store. A write transaction's
// puts and removes publish with its commit and drop with its discard.
func (t *Tx) GetBlockStore() block.StoreOps {
	return &t.blocks
}

// WroteBlocks reports whether the transaction put or removed a block.
func (t *Tx) WroteBlocks() bool {
	t.s.mtx.Lock()
	defer t.s.mtx.Unlock()
	return len(t.own) != 0
}

// Commit publishes the transaction's records and blocks with the pending
// blocks in one durable index commit.
func (t *Tx) Commit(ctx context.Context) error {
	return t.commit(ctx, false)
}

// CommitOrdered commits with write ordering only, unless a block put waits
// for publication. Publishing a block put needs a durable commit.
func (t *Tx) CommitOrdered(ctx context.Context) error {
	return t.commit(ctx, true)
}

// commit publishes the transaction and ends it, with write ordering only if
// ordered is set and no block put waits.
func (t *Tx) commit(ctx context.Context, ordered bool) error {
	switch {
	case t.itx == nil:
		return kvtx.ErrDiscarded
	case !t.write:
		return kvtx.ErrNotWrite
	}
	defer t.Discard()
	return t.s.publish(ctx, t.itx, t.own, ordered)
}

// Discard ends the transaction, dropping its records and blocks.
func (t *Tx) Discard() {
	// End the index transaction once, drop the blocks, and release the writer.
	if t.itx == nil {
		return
	}
	t.itx.Discard()
	t.s.mtx.Lock()
	t.itx, t.own = nil, nil
	t.s.mtx.Unlock()
	t.unlock()
}

// _ is a type assertion
var _ kvtx.OrderedCommitTx = (*Tx)(nil)
