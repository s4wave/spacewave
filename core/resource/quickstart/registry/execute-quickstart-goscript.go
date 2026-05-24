//go:build goscript

package resource_quickstart_registry

import (
	"context"

	s4wave_quickstart_registry "github.com/s4wave/spacewave/sdk/quickstart/registry"
)

// ExecuteQuickstart runs a registered Quickstart seed handler against a mounted Space.
func (r *QuickstartRegistryResource) ExecuteQuickstart(
	ctx context.Context,
	req *s4wave_quickstart_registry.ExecuteQuickstartRequest,
) (*s4wave_quickstart_registry.ExecuteQuickstartResponse, error) {
	quickstartID := req.GetQuickstartId()
	if quickstartID == "" {
		return nil, ErrQuickstartIdRequired
	}
	if req.GetSpaceResourceId() == 0 {
		return nil, ErrSpaceResourceIdRequired
	}
	if r.LookupRegistration(quickstartID) == nil {
		return nil, ErrQuickstartNotRegistered
	}
	return nil, ErrQuickstartExecutionUnavailable
}
