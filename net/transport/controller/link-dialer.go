package transport_controller

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/keyed"
	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/sirupsen/logrus"
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
	// le reports dial progress.
	le *logrus.Entry
	// waitTransport waits for a ready transport.
	waitTransport func(context.Context) (transport.Transport, error)
	// publishLink records a returned link or waits for an incoming peer link.
	publishLink func(context.Context, peer.ID, *ccontainer.CContainer[link.Link], link.Link) error
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
	ld := &linkDialer{le: c.le, waitTransport: c.GetTransport, publishLink: c.publishDialedLink, key: key, opts: promise.NewPromise[*dialer.DialerOpts]()}
	ld.lnk = ccontainer.NewCContainer[link.Link](nil)
	return ld.executeLinkDialer, ld
}

// executeLinkDialer executes the link dialer.
func (l *linkDialer) executeLinkDialer(
	ctx context.Context,
) error {
	// Wait for the transport needed by this dial attempt.
	tpt, err := l.waitTransport(ctx)
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
	dialer := dialer.NewDialer(l.le, tptDialer, dialOpts, l.key.peerID, l.key.dialAddress)
	lnk, err := dialer.Execute(subCtx)

	// Normalize cancellation before returning dial errors.
	if ctx.Err() != nil {
		return context.Canceled
	}
	if err != nil {
		return err
	}

	// Record the dial result through the link index owner's publication operation.
	return l.publishLink(ctx, l.key.peerID, l.lnk, lnk)
}

// awaitLink waits until this dial request has a live link.
func (l *linkDialer) awaitLink(ctx context.Context) (link.Link, error) {
	return l.lnk.WaitValue(ctx, nil)
}

// clearLink forgets a lost link and reports whether it belonged to this dialer.
// The controller's link index lock must be held by the caller.
func (l *linkDialer) clearLink(lnk link.Link) bool {
	if l.lnk.GetValue() != lnk {
		return false
	}
	l.lnk.SetValue(nil)
	return true
}

// acquireLinkDialer holds an address dial request and supplies its options.
func (c *Controller) acquireLinkDialer(peerID peer.ID, address string, opts *dialer.DialerOpts) (*linkDialer, func()) {
	ref, dial, _ := c.linkDialers.AddKeyRef(linkDialerKey{peerID: peerID, dialAddress: address})
	dial.opts.SetResult(opts, nil)
	return dial, ref.Release
}

// publishDialedLink records a dial result while excluding concurrent link removal.
func (c *Controller) publishDialedLink(ctx context.Context, peerID peer.ID, result *ccontainer.CContainer[link.Link], lnk link.Link) error {
	// Incoming dials publish through the transport handler instead of returning a link.
	// Retain that local link so its loss restarts this standing dial request.
	if lnk == nil {
		for {
			var waitCh <-chan struct{}
			c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
				waitCh = getWaitCh()
				if links := c.linksByPeerID[peerID]; len(links) != 0 {
					lnk = links[0].lnk
					// Publish under the owner lock so link removal cannot miss this dialer.
					result.SetValue(lnk)
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
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		result.SetValue(lnk)
	})
	return nil
}

// _ is a type assertion.
var _ keyed.Routine = (*linkDialer)(nil).executeLinkDialer
