package stream_srpc_server

import (
	"context"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
)

// mountedStreamResolver resolves HandleMountedStream.
type mountedStreamResolver struct {
	c *Server
}

func newMountedStreamResolver(c *Server) *mountedStreamResolver {
	return &mountedStreamResolver{c: c}
}

// Resolve resolves the values, emitting them to the handler.
// The resolver may be canceled and restarted multiple times.
// Any fatal error resolving the value is returned.
// The resolver will not be retried after returning an error.
// Values will be maintained from the previous call.
func (r *mountedStreamResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	var val link.MountedStreamHandler = r.c
	_, _ = handler.AddValue(val)
	return nil
}

// _ is a type assertion
var _ directive.Resolver = ((*mountedStreamResolver)(nil))
