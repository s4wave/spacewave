package sobject_world_engine

import (
	"context"

	"github.com/aperturerobotics/util/refcount"
	"github.com/s4wave/spacewave/db/block"
)

// deadlistPreparation owns one bounded record preparation. The ledger, the
// active worker and every admitted naming receipt own independent stage refs.
// Cancelling an engine or orphaning an entry cannot release admitted bytes.
type deadlistPreparation struct {
	// ctx cancels writes after orphaning or engine retirement.
	ctx context.Context
	// stage retains prepared blocks independently of the engine lifetime.
	stage *refcount.RefCount[block.StoreOps]
	// ledger is the ledger's ownership until durable naming or abandonment.
	ledger *refcount.Ref[block.StoreOps]
	// cancel stops only preparation I/O, never an admitted naming receipt.
	cancel context.CancelFunc
	// done publishes err after all preparation writes end.
	done chan struct{}
	// err is immutable after done closes.
	err error
}

// abandon releases only ledger ownership, leaving workers and receipts intact.
// The controller writer serializes calls and receipt-reference acquisition.
func (p *deadlistPreparation) abandon() {
	// An entry may have been forfeited before preparation started.
	if p == nil {
		return
	}

	// Stop only this job and return the ledger's independent reference.
	p.cancel()
	if p.ledger != nil {
		p.ledger.Release()
		p.ledger = nil
	}
}
