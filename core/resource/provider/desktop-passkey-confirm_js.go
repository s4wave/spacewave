//go:build js

package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// ConfirmDesktopPasskey is unavailable in the browser runtime.
func (s *SpacewaveProviderResource) ConfirmDesktopPasskey(
	ctx context.Context,
	req *s4wave_provider_spacewave.ConfirmDesktopPasskeyRequest,
) (*s4wave_provider_spacewave.ConfirmDesktopPasskeyResponse, error) {
	_ = s
	_ = ctx
	_ = req
	return nil, errors.New("desktop passkey confirm is only available in native builds")
}
