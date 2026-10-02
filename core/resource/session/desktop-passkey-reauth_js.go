//go:build js

package resource_session

import (
	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// StartDesktopPasskeyReauth is unavailable in the browser runtime. Web clients
// run the inline passkey reauth ceremony inside the renderer and call the
// existing auth endpoints directly.
func (r *SpacewaveSessionResource) StartDesktopPasskeyReauth(
	_ *s4wave_provider_spacewave.StartDesktopPasskeyReauthRequest,
	_ s4wave_session.SRPCSpacewaveSessionResourceService_StartDesktopPasskeyReauthStream,
) error {
	return errors.New("desktop passkey reauth is only available on native builds")
}
