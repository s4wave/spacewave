package cdn_world_controller

import (
	"context"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/ccontainer"
	block_store "github.com/s4wave/spacewave/db/block/store"
)

// blockStoreAuthority keeps a published store open until every resolver has
// withdrawn it. The Controller creates one authority for each Execute attempt.
type blockStoreAuthority struct {
	store block_store.Store

	mtx       sync.Mutex
	accepting bool
	refs      int
	drained   chan struct{}
}

func newBlockStoreAuthority(store block_store.Store) *blockStoreAuthority {
	return &blockStoreAuthority{
		store:     store,
		accepting: true,
		drained:   make(chan struct{}),
	}
}

func (a *blockStoreAuthority) acquire() bool {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if !a.accepting {
		return false
	}
	a.refs++
	return true
}

func (a *blockStoreAuthority) release() {
	a.mtx.Lock()
	defer a.mtx.Unlock()
	if a.refs == 0 {
		return
	}
	a.refs--
	if !a.accepting && a.refs == 0 {
		close(a.drained)
	}
}

func (a *blockStoreAuthority) withdraw() {
	a.mtx.Lock()
	if a.accepting {
		a.accepting = false
		if a.refs == 0 {
			close(a.drained)
		}
	}
	a.mtx.Unlock()
}

func (a *blockStoreAuthority) wait() {
	<-a.drained
}

type blockStoreResolver struct {
	ctr *ccontainer.CContainer[*blockStoreAuthority]
}

func (r *blockStoreResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	var current *blockStoreAuthority
	var valueID uint32
	var release, clearCallback func()
	clearValue := func() {
		if clearCallback != nil {
			clearCallback()
			clearCallback = nil
		}
		if valueID != 0 {
			_, _ = handler.RemoveValue(valueID)
			valueID = 0
		}
		if release != nil {
			release()
			release = nil
		}
	}
	defer func() {
		clearValue()
		handler.ClearValues()
	}()

	for {
		next, err := r.ctr.WaitValueChange(ctx, current, nil)
		if err != nil {
			return err
		}
		clearValue()
		current = next
		if current == nil || !current.acquire() {
			continue
		}
		release = sync.OnceFunc(current.release)
		id, accepted := handler.AddValue(current.store)
		if !accepted {
			release()
			release = nil
			continue
		}
		valueID = id
		clearCallback = handler.AddValueRemovedCallback(valueID, release)
		handler.MarkIdle(true)
	}
}

// _ is a type assertion
var _ directive.Resolver = (*blockStoreResolver)(nil)
