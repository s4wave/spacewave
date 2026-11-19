package sobject_world_engine

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
)

// durableHead is the World readers see. Writers fork from the admitted World,
// which the engine installs at admission. Each install queues its World here,
// and the durable head installs it once the publication admitted before it,
// if any, succeeds, in install order. A World after a failed publication is
// dropped: the reinstalled World follows it.
type durableHead struct {
	// promoteMtx serializes the installs of the durable head.
	promoteMtx sync.Mutex
	// bcast guards the fields below and broadcasts when done advances.
	bcast broadcast.Broadcast
	// queue holds the Worlds not yet installed or dropped, in install order.
	queue []*durableInstall
	// queued is the position of the last queued World, and last is that World.
	queued uint64
	last   *durableInstall
	// done is the position of the last World installed or dropped.
	done uint64
}

// durableInstall is a World the durable head installs after a receipt.
type durableInstall struct {
	// pos is the position of the install.
	pos uint64
	// after is the receipt of the last publication admitted before the World
	// was installed for writers, or nil when none was pending.
	after *block.PublicationReceipt
	// ref is the World head.
	ref *bucket.ObjectRef
	// dropped is set when the durable head dropped the World after a failed
	// receipt, and seqno is then the seqno of the World readers kept. Both
	// are set before done reaches pos.
	dropped bool
	seqno   uint64
}

// splitHeads reports whether readers and writers use separate block engines.
func (e *soEngine) splitHeads() bool {
	return e.admitted != e.bengine
}

// queueDurable queues ref for the durable head after the last admitted
// publication and installs what it can. The caller holds the writer lock and
// installed ref for writers.
func (e *soEngine) queueDurable(ref *bucket.ObjectRef) {
	if !e.splitHeads() {
		return
	}
	after := e.lastReceipt()
	e.durable.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		e.durable.queued++
		e.durable.last = &durableInstall{pos: e.durable.queued, after: after, ref: ref}
		e.durable.queue = append(e.durable.queue, e.durable.last)
	})
	e.promoteDurable()
}

// lastDurable returns the last queued World, or nil when the engine has a
// single head or queued none.
func (e *soEngine) lastDurable() *durableInstall {
	var last *durableInstall
	e.durable.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		last = e.durable.last
	})
	return last
}

// queuedDurableRoots returns the World roots queued for the durable head and
// not yet installed or dropped. The page view keeps their pages until the
// durable head pins them.
func (e *soEngine) queuedDurableRoots() []*block.BlockRef {
	var roots []*block.BlockRef
	e.durable.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, inst := range e.durable.queue {
			roots = append(roots, inst.ref.GetRootRef())
		}
	})
	return roots
}

// promoteDurable installs the queued Worlds whose receipts resolved, in
// order, dropping those after a failed receipt. It stops at the first
// unresolved receipt, or at an install error, which it logs; the next
// resolved receipt retries. It ends with the engine's lifetime.
func (e *soEngine) promoteDurable() {
	ctx := e.lifetime
	e.durable.promoteMtx.Lock()
	defer e.durable.promoteMtx.Unlock()
	for {
		// Take the oldest queued World.
		var next *durableInstall
		e.durable.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if len(e.durable.queue) != 0 {
				next = e.durable.queue[0]
			}
		})
		if next == nil {
			return
		}

		// Install it once its receipt succeeds, or drop it and record the
		// seqno of the World readers keep.
		if next.after != nil {
			select {
			case <-next.after.Done():
			default:
				return
			}
		}
		dropped := next.after != nil && next.after.Err() != nil
		var seqno uint64
		if dropped {
			var err error
			seqno, err = e.bengine.GetSeqno(ctx)
			if err != nil {
				if ctx.Err() == nil {
					e.c.le.WithError(err).Warn("failed to read the durable World seqno")
				}
				return
			}
		} else if !e.bengine.GetRootRef().EqualVT(next.ref) {
			if err := e.bengine.SetRootRef(ctx, next.ref); err != nil {
				if ctx.Err() == nil {
					e.c.le.WithError(err).Warn("failed to install the durable World")
				}
				return
			}
		}
		e.durable.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			next.dropped, next.seqno = dropped, seqno
			e.durable.queue = e.durable.queue[1:]
			e.durable.done = next.pos
			broadcast()
		})
	}
}

// waitDurable waits until the durable head installed or dropped inst. A nil
// inst has nothing to wait for. It reports whether inst was dropped and the
// seqno of the World readers then kept.
func (e *soEngine) waitDurable(ctx context.Context, inst *durableInstall) (bool, uint64, error) {
	if inst == nil {
		return false, 0, nil
	}
	e.promoteDurable()
	for {
		var wait <-chan struct{}
		var dropped bool
		var seqno uint64
		e.durable.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if e.durable.done < inst.pos {
				wait = getWaitCh()
			} else {
				dropped, seqno = inst.dropped, inst.seqno
			}
		})
		if wait == nil {
			return dropped, seqno, nil
		}
		select {
		case <-ctx.Done():
			return false, 0, context.Cause(ctx)
		case <-wait:
		}
	}
}
