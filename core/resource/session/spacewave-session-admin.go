package resource_session

import (
	"context"

	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// GetAdminJson reads one cloud administration route as the session's account.
func (r *SpacewaveSessionResource) GetAdminJson(
	ctx context.Context,
	req *s4wave_session.GetAdminJsonRequest,
) (*s4wave_session.GetAdminJsonResponse, error) {
	// Resolve the current signer before the cloud request.
	cli, err := r.swAcc.GetSessionClient()
	if err != nil {
		return nil, err
	}

	// Read the requested admin resource through the current signer.
	body, err := cli.GetAdminJSON(ctx, req.GetPath())
	if err != nil {
		return nil, err
	}
	return &s4wave_session.GetAdminJsonResponse{Body: string(body)}, nil
}
