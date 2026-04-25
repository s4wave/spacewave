//go:build !js

package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	"github.com/s4wave/spacewave/core/session"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartBrowserHandoff opens the browser auth handoff flow on native builds.
func (s *SpacewaveProviderResource) StartBrowserHandoff(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartBrowserHandoffRequest,
) (*s4wave_provider_spacewave.StartBrowserHandoffResponse, error) {
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(
		ctx,
		s.b,
		"",
		false,
		nil,
	)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	clientType := req.GetClientType()
	if clientType == "" {
		clientType = "desktop"
	}

	sessionPriv, accountID, entityID, err := provider_spacewave_handoff.StartHandoff(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		s.provider.GetPublicBaseURL(),
		clientType,
		req.GetAuthIntent(),
		req.GetUsername(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "start browser handoff")
	}

	listEntry, err := s.provider.MountHandoffSession(
		ctx,
		accountID,
		entityID,
		sessionPriv,
		sessionCtrl,
	)
	if err != nil {
		return nil, errors.Wrap(err, "mount handed-off session")
	}

	return &s4wave_provider_spacewave.StartBrowserHandoffResponse{
		SessionListEntry: listEntry,
	}, nil
}
