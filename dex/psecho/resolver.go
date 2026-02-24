package psecho

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	"github.com/aperturerobotics/hydra/dex"
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
	ref := dir.LookupBlockFromNetworkRef()
	if ref.GetEmpty() {
		return nil, nil
	}
	return directive.Resolvers(&lookupResolver{c: c, ref: ref}), nil
}

// Resolve resolves the values, emitting them to the handler.
func (r *lookupResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	refStr := r.ref.MarshalString()

	// Add to wantlist.
	r.c.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
		r.c.wantRefs[refStr] = r.ref
		bcast()
	})

	// Trigger immediate publish.
	r.c.publishNow.Store(1)

	// Wait for block to arrive.
	for {
		var ch <-chan struct{}
		var found bool
		r.c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			_, found = r.c.wantRefs[refStr]
		})

		// Block was removed from wantlist: it was received.
		if !found {
			// Look up the data from the local bucket.
			lk, rel, err := r.c.getBucketLookup(ctx)
			if err != nil {
				return err
			}
			if lk == nil {
				handler.AddValue(dex.NewLookupBlockFromNetworkValue(nil, nil))
				return nil
			}
			data, ok, err := lk.LookupBlock(ctx, r.ref, WithLocalOnly())
			rel()
			if err != nil {
				return err
			}
			if !ok {
				handler.AddValue(dex.NewLookupBlockFromNetworkValue(nil, nil))
				return nil
			}
			handler.AddValue(dex.NewLookupBlockFromNetworkValue(data, nil))
			return nil
		}

		select {
		case <-ctx.Done():
			// Remove from wantlist on cancel.
			r.c.bcast.HoldLock(func(bcast func(), _ func() <-chan struct{}) {
				delete(r.c.wantRefs, refStr)
				bcast()
			})
			return ctx.Err()
		case <-ch:
		}
	}
}

// _ is a type assertion
var _ directive.Resolver = ((*lookupResolver)(nil))
