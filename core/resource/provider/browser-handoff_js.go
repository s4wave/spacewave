//go:build js

package resource_provider

import (
	"context"

	"github.com/pkg/errors"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
)

// StartBrowserHandoff is unavailable in the browser runtime.
func (s *SpacewaveProviderResource) StartBrowserHandoff(
	ctx context.Context,
	req *s4wave_provider_spacewave.StartBrowserHandoffRequest,
) (*s4wave_provider_spacewave.StartBrowserHandoffResponse, error) {
	return nil, errors.New("browser auth handoff is only available on native builds")
}
