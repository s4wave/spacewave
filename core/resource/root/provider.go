package resource_root

import (
	"context"

	"github.com/pkg/errors"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/provider"
	resource_provider "github.com/s4wave/spacewave/core/resource/provider"
	s4wave_root "github.com/s4wave/spacewave/sdk/root"
)

// LookupProvider accesses a provider Resource by ID.
func (s *CoreRootServer) LookupProvider(ctx context.Context, req *s4wave_root.LookupProviderRequest) (*s4wave_root.LookupProviderResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	prov, providerRef, err := provider.ExLookupProvider(ctx, s.b, req.GetProviderId(), false, nil)
	if err != nil {
		return nil, err
	}
	if prov == nil {
		providerRef.Release()
		return nil, errors.New("provider not found")
	}

	providerResource := resource_provider.NewProviderResource(s.le, s.b, prov)
	id, err := resourceCtx.AddResource(providerResource.GetMux(), providerRef.Release)
	if err != nil {
		providerRef.Release()
		return nil, err
	}

	return &s4wave_root.LookupProviderResponse{ResourceId: id}, nil
}

// ListProviders lists the available providers.
func (s *CoreRootServer) ListProviders(
	ctx context.Context,
	req *s4wave_root.ListProvidersRequest,
) (*s4wave_root.ListProvidersResponse, error) {
	infos, err := provider.ExLookupProviderInfos(ctx, s.b, "", true)
	if err != nil {
		return nil, err
	}
	return &s4wave_root.ListProvidersResponse{Providers: infos}, nil
}
