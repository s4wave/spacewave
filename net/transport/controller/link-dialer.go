package transport_controller

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
)

// linkDialerKey is the peer ID and link address tuple.
type linkDialerKey struct {
	peerID      peer.ID
	dialAddress string
}

// linkDialer is a link dialer instance.
type linkDialer struct {
	// c is the controller
	c *Controller
	// key is the link dialer key
	key linkDialerKey
	// opts contains the link dialer opts
	// resolved by the caller who created this dialer
	opts *promise.Promise[*dialer.DialerOpts]
	// lnk is the link that resolved by this dialer
	lnk *ccontainer.CContainer[link.Link]
}

// buildLinkDialer constructs a new link dialer.
func (c *Controller) buildLinkDialer(key linkDialerKey) (keyed.Routine, *linkDialer) {
	// Allocate the keyed dialer state and result container.
	ld := &linkDialer{c: c, key: key, opts: promise.NewPromise[*dialer.DialerOpts]()}
	ld.lnk = ccontainer.NewCContainer[link.Link](nil)
	return ld.executeLinkDialer, ld
}

// executeLinkDialer executes the link dialer.
func (l *linkDialer) executeLinkDialer(
	ctx context.Context,
) error {
	// Wait for the transport needed by this dial attempt.
	tpt, err := l.c.GetTransport(ctx)
	if err != nil {
		return err
	}

	// Require a transport implementation that can dial peers.
	tptDialer, ok := tpt.(dialer.TransportDialer)
	if !ok {
		return dialer.ErrNotTransportDialer
	}

	// Wait for the caller to provide dial options.
	dialOpts, err := l.opts.Await(ctx)
	if err != nil {
		return err
	}

	// Scope the dial attempt to the routine context.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	// Execute the dial with the resolved address and peer.
	dialer := dialer.NewDialer(l.c.le, tptDialer, dialOpts, l.key.peerID, l.key.dialAddress)
	lnk, err := dialer.Execute(subCtx)

	// Normalize cancellation before returning dial errors.
	if ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		return err
	}

	// Publish the established link to waiters.
	l.lnk.SetValue(lnk)
	return nil
}

// _ is a type assertion
var _ keyed.Routine = (*linkDialer)(nil).executeLinkDialer
