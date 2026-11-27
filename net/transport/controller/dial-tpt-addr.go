package transport_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/tptaddr"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// dialTptAddrResolver resolves DialTptAddr directives.
type dialTptAddrResolver struct {
	// waitTransport waits for the transport to become ready.
	waitTransport func(context.Context) (transport.Transport, error)
	// acquireDialer holds an address dial request and supplies its options.
	acquireDialer func(peer.ID, string, *dialer.DialerOpts) (*linkDialer, func())
	// dir holds the address and peer constraints.
	dir tptaddr.DialTptAddr
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (o *dialTptAddrResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Wait for the transport that can resolve this address directive.
	tpt, err := o.waitTransport(ctx)
	if err != nil {
		return err
	}

	// Resolve addresses only through transports that support dialing.
	tptDialer, ok := tpt.(dialer.TransportDialer)
	if !ok {
		return nil
	}

	// Require the address directive to target this transport source peer.
	tptPeerID := tpt.GetPeerID()
	if srcPeerID := o.dir.DialTptAddrSourcePeerId(); srcPeerID != tptPeerID {
		// tpt peer id mismatch
		return nil
	}

	// Validate the destination peer and exclude dialing the local peer.
	destPeerID := o.dir.DialTptAddrTargetPeerId()
	if tptPeerID == destPeerID {
		// self dial
		return nil
	}
	if err := destPeerID.Validate(); err != nil {
		return err
	}

	// Decode the address and match its transport type before dialing.
	dialerOpts := o.dir.DialTptAddrDialerOpts()
	transportID, dialAddr, err := tptaddr.ParseTptAddr(dialerOpts.GetAddress())
	if err != nil {
		return nil
	}
	if !tptDialer.MatchTransportType(transportID) {
		return nil
	}

	// Reference the destination address dialer until resolution finishes.
	dialer, release := o.acquireDialer(destPeerID, dialAddr, dialerOpts)
	defer release()

	// wait for dialer to finish
	lnk, err := dialer.awaitLink(ctx)
	if err != nil {
		return err
	}

	// push the result
	var value tptaddr.DialTptAddrValue = lnk //nolint:staticcheck
	_, _ = handler.AddValue(value)
	return nil
}

// resolveDialTptAddr returns a resolver for dialing a transport address.
func (c *Controller) resolveDialTptAddr(
	_ context.Context,
	_ directive.Instance,
	dir tptaddr.DialTptAddr,
) ([]directive.Resolver, error) {
	// Require a destination peer and address before resolving the dial directive.
	srcPeerID := dir.DialTptAddrSourcePeerId()
	destPeerID := dir.DialTptAddrTargetPeerId()
	if len(destPeerID) == 0 || dir.DialTptAddrDialerOpts().GetAddress() == "" {
		return nil, nil
	}

	// Try to skip resolution if the directive does not match the transport
	// and the controller can lock. Otherwise return a resolver and check
	// later.
	var skip bool
	_ = c.bcast.TryHoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		if c.tpt != nil {
			tptPeerID := c.tpt.GetPeerID()
			skip = (tptPeerID == destPeerID) || (srcPeerID != "" && tptPeerID != srcPeerID)
		}
	})
	if skip {
		return nil, nil
	}

	// Return a resolver.
	return directive.Resolvers(&dialTptAddrResolver{
		waitTransport: c.GetTransport,
		acquireDialer: c.acquireLinkDialer,
		dir:           dir,
	}), nil
}

// _ is a type assertion
var _ directive.Resolver = (*dialTptAddrResolver)(nil)
