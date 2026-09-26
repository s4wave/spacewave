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
	// peerID is the authenticated destination identity.
	peerID peer.ID
	// dialAddress identifies the transport endpoint.
	dialAddress string
}

// linkDialer is a link dialer instance.
type linkDialer struct {
	// c owns the live links and dialer lifecycle.
	c *Controller
	// key identifies the requested peer and address.
	key linkDialerKey
	// opts is resolved by the caller that requested this dialer.
	opts *promise.Promise[*dialer.DialerOpts]
	// lnk records the local link whose loss must restart this dialer.
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

	// Incoming dials publish through the transport handler instead of returning a link.
	// Retain that local link so its loss restarts this standing dial request.
	if lnk == nil {
		for {
			var waitCh <-chan struct{}
			l.c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				waitCh = getWaitCh()
				if links := l.c.linksByPeerID[l.key.peerID]; len(links) != 0 {
					lnk = links[0].lnk
					// Publish under the owner lock so link removal cannot miss this dialer.
					l.lnk.SetValue(lnk)
				}
			})
			if lnk != nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-waitCh:
			}
		}
	}

	// Publish an outgoing link returned directly by the transport.
	// Publish under the owner lock so link removal cannot interleave.
	l.c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.lnk.SetValue(lnk)
	})
	return nil
}

// _ is a type assertion
var _ keyed.Routine = (*linkDialer)(nil).executeLinkDialer
