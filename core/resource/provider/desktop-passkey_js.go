//go:build js

package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartDesktopPasskey is unavailable in the browser runtime.
func (s *SpacewaveProviderResource) StartDesktopPasskey(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartDesktopPasskeyRequest,
) (*s4wave_provider_spacewave.StartDesktopPasskeyResponse, error) {
	_ = s
	_ = ctx
	_ = req
	return nil, errors.New("desktop passkey is only available in native builds")
}
