package resource_root

import (
	"context"

	"github.com/s4wave/spacewave/core/changelog"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// GetChangelog returns the embedded application changelog.
func (s *CoreRootServer) GetChangelog(
	ctx context.Context,
	req *s4wave_root.GetChangelogRequest,
) (*s4wave_root.GetChangelogResponse, error) {
	cl, err := changelog.GetChangelog()
	if err != nil {
		return nil, err
	}
	return &s4wave_root.GetChangelogResponse{Changelog: cl}, nil
}
