//go:build js

package resource_session

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartDesktopSSOLink is unavailable in the browser runtime. Web clients
// keep using the popup + BroadcastChannel link path until the follow-on
// web convergence phase.
func (r *SpacewaveSessionResource) StartDesktopSSOLink(
	_ context.Context,
	_ *s4wave_provider_spacewave.StartDesktopSSOLinkRequest,
) (*s4wave_provider_spacewave.StartDesktopSSOLinkResponse, error) {
	return nil, errors.New("desktop sso link is only available on native builds")
}
