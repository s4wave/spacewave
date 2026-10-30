package psecho

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/dex"
)

// lookupResolver resolves a LookupBlockFromNetwork directive via want-list.
type lookupResolver struct {
	c   *Controller
	ref *block.BlockRef
}

// resolveLookupBlockFromNetwork resolves a LookupBlockFromNetwork directive.
func (c *Controller) resolveLookupBlockFromNetwork(
	_ context.Context,
	_ directive.Instance,
	dir dex.LookupBlockFromNetwork,
) ([]directive.Resolver, error) {
	// Resolve a lookup that names a block.
	ref := dir.LookupBlockFromNetworkRef()
	if ref.GetEmpty() {
		return nil, nil
	}
	return directive.Resolvers(&lookupResolver{c: c, ref: ref}), nil
}

// Resolve asks peers for the block and emits it once one sends it. The block
// is served and not stored: a peer sends no references, and a stored block
// without them would hide its children from the collector.
func (r *lookupResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Join the want for the block, adding it to the want list if new, and
	// leave it on return; the last waiter removes it.
	refStr := r.ref.MarshalString()
	r.c.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		w := r.c.wants[refStr]
		if w == nil {
			w = &want{ref: r.ref}
			r.c.wants[refStr] = w
		}
		w.waiters++
		bcast()
	})
	defer r.c.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		w := r.c.wants[refStr]
		w.waiters--
		if w.waiters == 0 {
			delete(r.c.wants, refStr)
			bcast()
		}
	})

	// Trigger immediate publish.
	r.c.publishNow.Store(1)

	// Wait for a peer to send the block.
	for {
		var ch <-chan struct{}
		var data []byte
		r.c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			data = r.c.wants[refStr].data
		})
		if data != nil {
			handler.AddValue(dex.NewLookupBlockFromNetworkValue(data, nil))
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = (*lookupResolver)(nil)
