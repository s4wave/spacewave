package resource_provider

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	resource_account "github.com/s4wave/spacewave/core/resource/account"
	s4wave_provider "github.com/s4wave/spacewave/sdk/provider"
	s4wave_provider_local "github.com/s4wave/spacewave/sdk/provider/local"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

// ProviderResource wraps a core provider for resource access.
type ProviderResource struct {
	mux      srpc.Invoker
	provider provider.Provider
}

// NewProviderResource creates a new ProviderResource.
func NewProviderResource(le *logrus.Entry, b bus.Bus, prov provider.Provider) *ProviderResource {
	provResource := &ProviderResource{
		provider: prov,
	}

	registrations := []func(srpc.Mux) error{
		func(mux srpc.Mux) error {
			return s4wave_provider.SRPCRegisterProviderResourceService(mux, provResource)
		},
	}

	switch p := prov.(type) {
	case *provider_spacewave.Provider:
		sw := NewSpacewaveProviderResource(provResource, le, b, p)
		registrations = append(registrations, func(mux srpc.Mux) error {
			return s4wave_provider_spacewave.SRPCRegisterSpacewaveProviderResourceService(mux, sw)
		})
	case *provider_local.Provider:
		local := NewLocalProviderResource(provResource, le, b, p)
		registrations = append(registrations, func(mux srpc.Mux) error {
			return s4wave_provider_local.SRPCRegisterLocalProviderResourceService(mux, local)
		})
	}

	provResource.mux = resource_server.NewResourceMux(registrations...)
	return provResource
}

// GetMux returns the rpc mux.
func (r *ProviderResource) GetMux() srpc.Invoker {
	return r.mux
}

// GetProviderInfo returns information about this provider.
func (r *ProviderResource) GetProviderInfo(ctx context.Context, req *s4wave_provider.GetProviderInfoRequest) (*s4wave_provider.GetProviderInfoResponse, error) {
	return &s4wave_provider.GetProviderInfoResponse{
		ProviderInfo: r.provider.GetProviderInfo(),
	}, nil
}

// AccessProviderAccount mounts a provider account and returns a resource ID.
func (r *ProviderResource) AccessProviderAccount(ctx context.Context, req *s4wave_provider.AccessProviderAccountRequest) (*s4wave_provider.AccessProviderAccountResponse, error) {
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	account, relFn, err := r.provider.AccessProviderAccount(ctx, req.GetAccountId(), nil)
	if err != nil {
		return nil, err
	}

	accResource := resource_account.NewAccountResource(account)
	var mux srpc.Invoker
	if accResource != nil {
		mux = accResource.GetMux()
	}
	releaseFn := func() {
		if accResource != nil {
			accResource.Release()
		}
		relFn()
	}
	id, err := resourceCtx.AddResource(mux, releaseFn)
	if err != nil {
		releaseFn()
		return nil, err
	}

	return &s4wave_provider.AccessProviderAccountResponse{ResourceId: id}, nil
}

// _ is a type assertion
var _ s4wave_provider.SRPCProviderResourceServiceServer = ((*ProviderResource)(nil))
