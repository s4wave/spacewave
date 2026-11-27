package transport_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// establishLinkResolver resolves establishLink directives.
type establishLinkResolver struct {
	// bus holds source-scoped peer link demand.
	bus bus.Bus
	// waitTransport waits for the transport to become ready.
	waitTransport func(context.Context) (transport.Transport, error)
	// acquireDialer holds an address dial request and supplies its options.
	acquireDialer func(peer.ID, string, *dialer.DialerOpts) (*linkDialer, func())
	// snapshotLinks copies a peer's links and subscribes to changes atomically.
	snapshotLinks func(peer.ID, []*registeredLink) ([]*registeredLink, <-chan struct{})
	// dir holds the requested source and destination peers.
	dir link.EstablishLinkWithPeer
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *establishLinkResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Wait for the transport that can establish the target peer link.
	targetPeerID := o.dir.EstablishLinkTargetPeerId()
	tpt, err := o.waitTransport(ctx)
	if err != nil {
		return err
	}

	// Keep a source-scoped link demand when this transport matches the directive.
	sourcePeerID := o.dir.EstablishLinkSourcePeerId()
	tptSourcePeerID := tpt.GetPeerID()
	if sourcePeerID == "" {
		// If the directive filtered only by destination peer ID, add a EstablishLink
		// directive for the source to ensure there is a reference.
		_, estLinkRef, err := o.bus.AddDirective(ctx, link.NewEstablishLinkWithPeer(tptSourcePeerID, targetPeerID), nil)
		if err != nil {
			return err
		}
		defer estLinkRef.Release()
	} else if sourcePeerID != tptSourcePeerID {
		// The source peer id does not match the transport peer ID.
		// Bail.
		return nil
	}

	// Determine if the transport can dial the peer at a specific address.
	var dialAddress string
	tptDialer, ok := tpt.(dialer.TransportDialer)
	if ok {
		dialerOpts, err := tptDialer.GetPeerDialer(ctx, targetPeerID)
		if err != nil {
			return err
		}

		dialAddress = dialerOpts.GetAddress()
		if dialerOpts.GetAddress() != "" {
			// Add a reference to request that the controller dial
			// that peer.
			_, release := o.acquireDialer(targetPeerID, dialAddress, dialerOpts)
			defer release()
		}
	}

	// Avoid churn by using a uniqueListResolver.
	uniqueListResolver := directive.NewUniqueListXfrmResolver(
		func(v *registeredLink) uint64 {
			return v.lnk.GetUUID()
		}, func(k uint64, a, b *registeredLink) bool {
			return a == b
		},
		func(k uint64, v *registeredLink) (link.EstablishLinkWithPeerValue, bool) {
			return v.mlnk, true
		},
		handler,
	)

	// Wait for the link(s) to be resolved.
	var waitCh <-chan struct{}
	var values []*registeredLink
	for {
		if ctx.Err() != nil {
			return context.Canceled
		}

		// reuse the slice
		values = values[:0]
		values, waitCh = o.snapshotLinks(targetPeerID, values)

		// update the unique list resolver, emitting values to the resolver
		// handler.
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
	_ context.Context,
	_ directive.Instance,
	dir link.EstablishLinkWithPeer,
) ([]directive.Resolver, error) {
	if len(dir.EstablishLinkTargetPeerId()) == 0 {
		return nil, nil
	}

	// Try to skip resolution if the directive does not match the transport
	// and the controller can lock. Otherwise return a resolver and check
	// later.
	if srcPeerID := dir.EstablishLinkSourcePeerId(); len(srcPeerID) != 0 {
		var skip bool
		_ = c.bcast.TryHoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			skip = c.tpt != nil && c.tpt.GetPeerID() != srcPeerID
		})
		if skip {
			return nil, nil
		}
	}

	// Return a resolver.
	return directive.Resolvers(&establishLinkResolver{
		bus:           c.bus,
		waitTransport: c.GetTransport,
		acquireDialer: c.acquireLinkDialer,
		snapshotLinks: c.snapshotPeerLinks,
		dir:           dir,
	}), nil
}

// _ is a type assertion
var _ directive.Resolver = (*establishLinkResolver)(nil)
