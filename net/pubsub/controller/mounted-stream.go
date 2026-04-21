package pubsub_controller

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
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
	ps, err := r.c.GetPubSub(ctx)
	if err != nil {
		return err
	}

	var val link.MountedStreamHandler = newStreamHandler(r.c, ps)
	_, _ = handler.AddValue(val)
	return nil
}

// handleMountedStream handles the HandleMountedStream directive.
func (c *Controller) handleMountedStream(
	ctx context.Context,
	di directive.Instance,
	dir link.HandleMountedStream,
) ([]directive.Resolver, error) {
	if dir.HandleMountedStreamProtocolID() != c.protocolID {
		return nil, nil
	}
	return directive.Resolvers(newHandleMountedStreamResolver(c)), nil
}

// _ is a type assertion
var _ directive.Resolver = ((*handleMountedStreamResolver)(nil))
