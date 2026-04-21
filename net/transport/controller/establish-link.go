package transport_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// establishLinkResolver resolves establishLink directives
type establishLinkResolver struct {
	c   *Controller
	ctx context.Context
	di  directive.Instance
	dir link.EstablishLinkWithPeer
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *establishLinkResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	targetPeerID := o.dir.EstablishLinkTargetPeerId()
	tpt, err := o.c.GetTransport(ctx)
	if err != nil {
		return err
	}

	sourcePeerID := o.dir.EstablishLinkSourcePeerId()
	tptSourcePeerID := tpt.GetPeerID()
	if sourcePeerID == "" {
		// If the directive filtered only by destination peer ID, add a EstablishLink
		// directive for the source to ensure there is a reference.
		_, estLinkRef, err := o.c.bus.AddDirective(link.NewEstablishLinkWithPeer(tptSourcePeerID, targetPeerID), nil)
		if err != nil {
			return err
		}
		defer estLinkRef.Release()
	} else if sourcePeerID != tptSourcePeerID {
		// The source peer id does not match our peer ID. Bail.
		return nil
	}

	// Determine if we can dial the peer at a specific address.
	var dialAddress string
	tptDialer, ok := tpt.(dialer.TransportDialer)
	if ok {
		dialerOpts, err := tptDialer.GetPeerDialer(ctx, targetPeerID)
		if err != nil {
			return err
		}

		dialAddress = dialerOpts.GetAddress()
		if dialerOpts.GetAddress() != "" {
			// Add a reference to request that we dial that peer.
			dialRef, dial, _ := o.c.linkDialers.AddKeyRef(linkDialerKey{peerID: targetPeerID, dialAddress: dialAddress})
			defer dialRef.Release()

			// Attempt to dial the peer at that address.
			dial.opts.SetResult(dialerOpts, nil)
		}
	}

	// Avoid churn by using a uniqueListResolver.
	uniqueListResolver := directive.NewUniqueListXfrmResolver(
		func(v *establishedLink) uint64 {
			return v.lnk.GetUUID()
		}, func(k uint64, a, b *establishedLink) bool {
			return a == b
		},
		func(k uint64, v *establishedLink) (link.EstablishLinkWithPeerValue, bool) {
			return v.mlnk, true
		},
		handler,
	)

	// Wait for the link(s) to be resolved.
	var waitCh <-chan struct{}
	var values []*establishedLink
	for {
		if ctx.Err() != nil {
			return context.Canceled
		}

		// reuse slice
		values = values[:0]
		o.c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			// get all links matching the target peer
			values = append(values, o.c.linksByPeerID[targetPeerID]...)

			// get wait ch to watch for changes
			waitCh = getWaitCh()
		})

		// update unique list resolver, emitting values to the resolver handler.
		uniqueListResolver.SetValues(values...)

		select {
		case <-ctx.Done():
			return context.Canceled
		case <-waitCh:
		}
	}
}

// resolveEstablishLink returns a resolver for opening a stream.
// Negotiates the protocol ID as well.
func (c *Controller) resolveEstablishLink(
	ctx context.Context,
	di directive.Instance,
	dir link.EstablishLinkWithPeer,
) ([]directive.Resolver, error) {
	if len(dir.EstablishLinkTargetPeerId()) == 0 {
		return nil, nil
	}

	// Try to skip this if it doesn't match this transport and we can lock.
	// Otherwise return a resolver and check later.
	if srcPeerID := dir.EstablishLinkSourcePeerId(); len(srcPeerID) != 0 {
		var skip bool
		_ = c.bcast.TryHoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			skip = c.tpt != nil && c.tpt.GetPeerID() != srcPeerID
		})
		if skip {
			return nil, nil
		}
	}

	// Return resolver.
	return directive.Resolvers(&establishLinkResolver{
		c:   c,
		ctx: ctx,
		di:  di,
		dir: dir,
	}), nil
}

// _ is a type assertion
var _ directive.Resolver = ((*establishLinkResolver)(nil))
