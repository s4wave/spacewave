package signaling_rpc_client

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/signaling"
)

// signalPeerResolver resolves SignalPeer.
type signalPeerResolver struct {
	c   *Controller
	dir signaling.SignalPeer
}

// resolveSignalPeer resolves the SignalPeer directive.
func (c *Controller) resolveSignalPeer(
	ctx context.Context,
	di directive.Instance,
	dir signaling.SignalPeer,
) ([]directive.Resolver, error) {
	// Check if the directive matches this client.
	if signalingID := dir.SignalingID(); signalingID != "" {
		if c.conf.GetSignalingId() != signalingID {
			return nil, nil
		}
	}

	if localPeerID := dir.SignalLocalPeerID(); localPeerID != "" {
		// Check if we know the local peer id yet.
		// We might not yet know if client is nil.
		// Return a resolver anyway in this case.
		if client := c.client.GetValue(); client != nil {
			if client.peerID.String() != localPeerID.String() {
				return nil, nil
			}
		}
	}

	// Return the resolver.
	return directive.R(&signalPeerResolver{
		c:   c,
		dir: dir,
	}, nil)
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *signalPeerResolver) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	// Wait for the client to be ready.
	handler.ClearValues()
	client, err := r.c.client.WaitValue(ctx, nil)
	if err != nil {
		return err
	}

	if localPeerID := r.dir.SignalLocalPeerID(); localPeerID != "" {
		if client.peerID.String() != localPeerID.String() {
			// Peer ID mismatch
			return nil
		}
	}

	// Add reference
	remotePeerIDStr := r.dir.SignalRemotePeerID().String()
	peerRef := client.AddPeerRef(remotePeerIDStr)

	// Emit value
	var val signaling.SignalPeerValue = NewSessionWithRef(peerRef)
	vid, accepted := handler.AddValue(val)
	if !accepted {
		// We must already have a signaling channel via some other approach.
		// Drop this one.
		peerRef.Release()
		return nil
	}

	// Release the ref when the value is removed or directive is released.
	handler.AddValueRemovedCallback(vid, peerRef.Release)

	// Done
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*signalPeerResolver)(nil))
