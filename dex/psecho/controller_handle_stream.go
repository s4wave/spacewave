package psecho

import (
	"context"

	"github.com/aperturerobotics/bifrost/link"
	"github.com/aperturerobotics/controllerbus/directive"
)

// mountedStreamResolver resolves incoming streams
type mountedStreamResolver struct {
	c   *Controller
	di  directive.Instance
	dir link.HandleMountedStream
}

// resolveHandleMountedStream resolves a lookup block from network directive.
func (c *Controller) resolveHandleMountedStream(
	ctx context.Context,
	di directive.Instance,
	dir link.HandleMountedStream,
) (directive.Resolver, error) {
	if dir.HandleMountedStreamProtocolID() != syncProtocolID {
		return nil, nil
	}
	return &mountedStreamResolver{
		c:   c,
		di:  di,
		dir: dir,
	}, nil
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *mountedStreamResolver) Resolve(
	ctx context.Context, handler directive.ResolverHandler,
) error {
	// Verify peer ID matches
	cs, err := r.c.getCState(ctx)
	if err != nil {
		return err
	}

	if cs.peerID.Pretty() == r.dir.HandleMountedStreamLocalPeerID().Pretty() {
		_, _ = handler.AddValue(r.c)
	}

	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*mountedStreamResolver)(nil))
