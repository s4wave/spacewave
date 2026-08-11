package cdn_world_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/ccontainer"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// blockStoreAuthority keeps a published store open until every resolver has
// withdrawn it. The Controller creates one authority for each Execute attempt.
type blockStoreAuthority struct {
	// store is the block-store value published to resolvers.
	store block_store.Store

	// bcast guards acceptance and resolver leases.
	bcast broadcast.Broadcast
	// accepting allows new resolver leases until Execute withdraws the store.
	accepting bool
	// leases are active resolver references to store.
	leases map[*blockStoreLease]struct{}
}

// blockStoreLease keeps one resolver's published store value alive.
type blockStoreLease struct {
	// authority records and releases this resolver lease.
	authority *blockStoreAuthority
}

func newBlockStoreAuthority(store block_store.Store) *blockStoreAuthority {
	return &blockStoreAuthority{
		store:     store,
		accepting: true,
		leases:    make(map[*blockStoreLease]struct{}),
	}
}

func (a *blockStoreAuthority) acquire() *blockStoreLease {
	var lease *blockStoreLease
	a.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !a.accepting {
			return
		}
		lease = &blockStoreLease{authority: a}
		a.leases[lease] = struct{}{}
	})
	return lease
}

func (l *blockStoreLease) release() {
	l.authority.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if _, ok := l.authority.leases[l]; !ok {
			return
		}
		delete(l.authority.leases, l)
		broadcast()
	})
}

func (a *blockStoreAuthority) withdraw() {
	a.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !a.accepting {
			return
		}
		a.accepting = false
		broadcast()
	})
}

func (a *blockStoreAuthority) wait() {
	for {
		var done bool
		var waitCh <-chan struct{}
		a.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			done = !a.accepting && len(a.leases) == 0
			if !done {
				waitCh = getWaitCh()
			}
		})
		if done {
			return
		}
		<-waitCh
	}
}

type blockStoreResolver struct {
	// ctr publishes the current store authority for the controller attempt.
	ctr *ccontainer.CContainer[*blockStoreAuthority]
}

// Resolve publishes each controller-owned block store until it is withdrawn.
func (r *blockStoreResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	var current *blockStoreAuthority
	var lease *blockStoreLease
	var valueID uint32
	var clearCallback func()
	clearValue := func() {
		if clearCallback != nil {
			clearCallback()
			clearCallback = nil
		}
		if valueID != 0 {
			_, _ = handler.RemoveValue(valueID)
			valueID = 0
		}
		if lease != nil {
			lease.release()
			lease = nil
		}
	}
	defer func() {
		clearValue()
		handler.ClearValues()
	}()

	for {
		// Replace the published value when the controller begins a new attempt.
		next, err := r.ctr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		clearValue()
		current = next
		if current == nil {
			continue
		}
		lease = current.acquire()
		if lease == nil {
			continue
		}

		// One idempotent lease covers explicit cleanup and handler removal.
		id, accepted := handler.AddValue(current.store)
		if !accepted {
			lease.release()
			lease = nil
			continue
		}
		valueID = id
		clearCallback = handler.AddValueRemovedCallback(valueID, lease.release)
		handler.MarkIdle(true)
	}
}

// _ is a type assertion
var _ directive.Resolver = (*blockStoreResolver)(nil)
