package psecho

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/dex"
)

// lookupBlockResolver resolves lookup block from network
type lookupBlockResolver struct {
	c   *Controller
	di  directive.Instance
	dir dex.LookupBlockFromNetwork
}

// resolveLookupBlockFromNetwork resolves a lookup block from network directive.
func (c *Controller) resolveLookupBlockFromNetwork(
	ctx context.Context,
	di directive.Instance,
	dir dex.LookupBlockFromNetwork,
) (directive.Resolver, error) {
	return &lookupBlockResolver{
		c:   c,
		di:  di,
		dir: dir,
	}, nil
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *lookupBlockResolver) Resolve(
	ctx context.Context, handler directive.ResolverHandler,
) error {
	ref := r.dir.LookupBlockFromNetworkRef()
	refStr := ref.MarshalString()
	r.c.mtx.Lock()
	bw, bwOk := r.c.waiters[refStr]
	if !bwOk {
		bw = newDesiredBlockWaiter(ref)
		r.c.waiters[refStr] = bw
	}
	bw.refcount++
	r.c.mtx.Unlock()

	if !bwOk {
		r.c.wakeExecute()
	}

	// wait for our routine to exit OR block to be found
	select {
	case <-ctx.Done():
		// decrement refcount
		r.c.mtx.Lock()
		bw.refcount--
		if bw.refcount == 0 {
			defer r.c.wakeExecute()
		}
		r.c.mtx.Unlock()
		return ctx.Err()
	case <-bw.doneCh:
		handler.AddValue(dex.NewLookupBlockFromNetworkValue(bw.data, bw.err))
		return nil
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupBlockResolver)(nil))
