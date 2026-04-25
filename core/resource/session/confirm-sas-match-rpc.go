package resource_session

import (
	"context"

	"github.com/pkg/errors"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// ConfirmSASMatch sends the user's SAS emoji verification decision to the
// bilateral confirmation exchange running over the bifrost link.
func (r *SessionResource) ConfirmSASMatch(ctx context.Context, req *s4wave_session.ConfirmSASMatchRequest) (*s4wave_session.ConfirmSASMatchResponse, error) {
	localAcc, ok := r.session.GetProviderAccount().(*provider_local.ProviderAccount)
	if !ok {
		return nil, errors.New("SAS match only supported for local provider")
	}

	localAcc.ConfirmSASMatch(req.GetConfirmed())
	return &s4wave_session.ConfirmSASMatchResponse{}, nil
}
