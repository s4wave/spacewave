package resource_session

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// LinkSpacePublicOrigin links an owner-paid bucket as a public Space's pack
// origin and starts copying its existing packs there.
func (r *SpacewaveSessionResource) LinkSpacePublicOrigin(
	ctx context.Context,
	req *s4wave_provider_spacewave.LinkSpacePublicOriginRequest,
) (*s4wave_provider_spacewave.LinkSpacePublicOriginResponse, error) {
	// Require the Space and its link.
	if req.GetSpaceId() == "" {
		return nil, errors.New("space id is required")
	}
	if req.GetLink() == nil {
		return nil, errors.New("public origin link is required")
	}

	// Link the origin through the session's cloud client.
	status, err := r.swAcc.GetSessionClient().LinkPublicOrigin(ctx, req.GetSpaceId(), req.GetLink())
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.LinkSpacePublicOriginResponse{Status: status}, nil
}

// GetSpacePublicOrigin returns a public Space's linked origin.
func (r *SpacewaveSessionResource) GetSpacePublicOrigin(
	ctx context.Context,
	req *s4wave_provider_spacewave.GetSpacePublicOriginRequest,
) (*s4wave_provider_spacewave.GetSpacePublicOriginResponse, error) {
	if req.GetSpaceId() == "" {
		return nil, errors.New("space id is required")
	}
	status, err := r.swAcc.GetSessionClient().GetPublicOrigin(ctx, req.GetSpaceId())
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.GetSpacePublicOriginResponse{Status: status}, nil
}

// ReleaseSpaceHostedCopies deletes the Spacewave-hosted copies of a Space that
// moved to its linked origin.
func (r *SpacewaveSessionResource) ReleaseSpaceHostedCopies(
	ctx context.Context,
	req *s4wave_provider_spacewave.ReleaseSpaceHostedCopiesRequest,
) (*s4wave_provider_spacewave.ReleaseSpaceHostedCopiesResponse, error) {
	if req.GetSpaceId() == "" {
		return nil, errors.New("space id is required")
	}
	status, err := r.swAcc.GetSessionClient().ReleaseHostedCopies(ctx, req.GetSpaceId())
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.ReleaseSpaceHostedCopiesResponse{Status: status}, nil
}
