//go:build js

package resource_session

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartDesktopPasskeyReauth is unavailable in the browser runtime. Web clients
// run the inline passkey reauth ceremony inside the renderer and call the
// existing auth endpoints directly.
func (r *SpacewaveSessionResource) StartDesktopPasskeyReauth(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartDesktopPasskeyReauthRequest,
) (*s4wave_provider_spacewave.StartDesktopPasskeyReauthResponse, error) {
	_ = ctx
	_ = req
	return nil, errors.New("desktop passkey reauth is only available on native builds")
}
