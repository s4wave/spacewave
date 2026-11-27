package paylog

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
)

// Domain is a key-value store under one prefix of the store's key-value
// space whose transactions also serve blocks. A write transaction's commit
// publishes its records and blocks together, and its discard drops both, so
// the domain is an atomic commit fence for both.
type Domain struct {
	// s is the store.
	s *Store
	// prefix is the domain's index key prefix.
	prefix []byte
}

// Domain returns the key-value store under prefix.
func (s *Store) Domain(prefix []byte) *Domain {
	return &Domain{s: s, prefix: append([]byte(kvPrefix), prefix...)}
}

// NewTransaction opens a transaction. A write transaction waits for the
// store's writer under ctx and holds it until Commit or Discard.
func (d *Domain) NewTransaction(ctx context.Context, write bool) (kvtx.Tx, error) {
	return d.s.begin(ctx, write, d.prefix)
}

// GetBlockStore returns the block store outside any transaction. Its puts and
// removes wait for the next durable commit or Sync.
func (d *Domain) GetBlockStore() block.StoreOps {
	return &d.s.blockStore
}

// Maintenance reclaims the space of removed blocks, as Store.Maintenance does.
func (d *Domain) Maintenance(ctx context.Context) error {
	return d.s.Maintenance(ctx)
}

// SupportsAtomicCommit reports that a transaction's Commit publishes its
// records and blocks synchronously and its Discard drops both.
func (d *Domain) SupportsAtomicCommit() bool {
	return true
}

// Sync makes every completed ordered commit durable, publishing the pending
// blocks.
func (d *Domain) Sync(ctx context.Context) error {
	return d.s.sync(ctx)
}

// WaitDurable makes every completed ordered commit durable. The index has no
// background flush to wait for, so WaitDurable syncs.
func (d *Domain) WaitDurable(ctx context.Context) error {
	return d.s.sync(ctx)
}

// WritersWaiting reports whether a write transaction waits for the writer.
func (d *Domain) WritersWaiting() bool {
	return d.s.waiting.Load() != 0
}

// _ is a type assertion
var (
	_ kvtx.AtomicCommitStore  = (*Domain)(nil)
	_ kvtx.OrderedCommitStore = (*Domain)(nil)
	_ kvtx.WriterQueueStore   = (*Domain)(nil)
)
