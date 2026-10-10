package spacewave_launcher_controller

import (
	"context"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// ReleaseAuthorityServer exposes this launcher's resolved distribution signer pins.
type ReleaseAuthorityServer struct {
	// c retains the immutable signer pins resolved when the launcher was constructed.
	c *Controller
}

// NewReleaseAuthorityServer constructs the release authority for a launcher.
func NewReleaseAuthorityServer(c *Controller) *ReleaseAuthorityServer {
	return &ReleaseAuthorityServer{c: c}
}

// GetReleasePeerIds supplies the exact pins used by the DistConfig verifier.
func (s *ReleaseAuthorityServer) GetReleasePeerIds(context.Context, *manifest.GetReleasePeerIdsRequest) (*manifest.GetReleasePeerIdsResponse, error) {
	// Encode the immutable pins for the resolver RPC.
	ids := make([]string, len(s.c.distPeerIDs))
	for i, id := range s.c.distPeerIDs {
		ids[i] = id.String()
	}
	return &manifest.GetReleasePeerIdsResponse{PeerIds: ids}, nil
}

// _ is a type assertion.
var _ manifest.SRPCReleaseAuthorityServer = (*ReleaseAuthorityServer)(nil)
