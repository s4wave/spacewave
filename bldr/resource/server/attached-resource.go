package resource_server

import (
	"context"

	"github.com/aperturerobotics/starpc/srpc"
)

// attachedResource is a client-provided resource accessible by server-side RPC handlers.
// Registered via ResourceAttach, keyed by server-assigned resource ID.
type attachedResource struct {
	// label is an informational description of the attached resource.
	label string
	// cancel cancels this resource's derived context without affecting the yamux session.
	cancel context.CancelFunc
	// srpcClient is the client for server-side handlers to invoke the attached resource.
	srpcClient srpc.Client
}
