package manifest_fetch_world

import (
	"context"

	manifest "github.com/s4wave/spacewave/bldr/manifest"
)

// releaseAuthorityTestServer provides immutable release pins over the real RPC transport.
type releaseAuthorityTestServer struct {
	// peers contains the release peer IDs supplied by the fixture launcher.
	peers []string
}

// GetReleasePeerIds supplies the fixture launcher's release pins.
func (s *releaseAuthorityTestServer) GetReleasePeerIds(context.Context, *manifest.GetReleasePeerIdsRequest) (*manifest.GetReleasePeerIdsResponse, error) {
	return &manifest.GetReleasePeerIdsResponse{PeerIds: s.peers}, nil
}

// _ is a type assertion.
var _ manifest.SRPCReleaseAuthorityServer = (*releaseAuthorityTestServer)(nil)
