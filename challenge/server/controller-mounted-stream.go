package auth_challenge_server

import (
	"context"

	auth_challenge "github.com/aperturerobotics/auth/challenge"
	"github.com/aperturerobotics/bifrost/link"
	"github.com/aperturerobotics/controllerbus/directive"
)

// handleMountedStreamResolver resolves HandleMountedStream.
type handleMountedStreamResolver struct {
	c *Controller
}

func newHandleMountedStreamResolver(c *Controller) *handleMountedStreamResolver {
	return &handleMountedStreamResolver{c: c}
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *handleMountedStreamResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	var val link.MountedStreamHandler = newStreamHandler(r.c)
	_, _ = handler.AddValue(val)
	return nil
}

// resolveHandleMountedStream handles the HandleMountedStream directive.
func (c *Controller) resolveHandleMountedStream(
	ctx context.Context,
	di directive.Instance,
	dir link.HandleMountedStream,
) (directive.Resolver, error) {
	switch {
	case dir.HandleMountedStreamProtocolID() != auth_challenge.ChallengeProtocolID:
		// protocol ID mismatch
	case c.peerID.Pretty() != dir.HandleMountedStreamLocalPeerID().Pretty():
		// peer ID mismatch
		c.le.
			WithField("incoming-peer-id", dir.HandleMountedStreamLocalPeerID().Pretty()).
			WithField("local-peer-id", c.peerID.Pretty()).
			Warn("skipping incoming auth stream due to peer id mismatch")
	default:
		return newHandleMountedStreamResolver(c), nil
	}

	return nil, nil
}

// _ is a type assertion
var _ directive.Resolver = ((*handleMountedStreamResolver)(nil))
