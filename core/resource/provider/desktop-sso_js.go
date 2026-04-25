//go:build js

package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartDesktopSSO is unavailable in the browser runtime.
func (s *SpacewaveProviderResource) StartDesktopSSO(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartDesktopSSORequest,
) (*s4wave_provider_spacewave.StartDesktopSSOResponse, error) {
	return nil, errors.New("desktop sso is only available on native builds")
}
