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
	// mux serves common and provider-specific Resource methods.
	mux srpc.Invoker
	// provider performs account operations for this Resource.
	provider provider.Provider
	// release drops provider-specific mounts when the Resource is released.
	release func()
}

// NewProviderResource creates a new ProviderResource.
func NewProviderResource(le *logrus.Entry, b bus.Bus, prov provider.Provider) *ProviderResource {
	// Construct the common Resource before registering provider-specific methods.
	provResource := &ProviderResource{
		provider: prov,
	}

	// Build one method mux for the common and concrete provider surfaces.
	registrations := []func(srpc.Mux) error{
		func(mux srpc.Mux) error {
			return s4wave_provider.SRPCRegisterProviderResourceService(mux, provResource)
		},
	}

	// Keep concrete resource lifetimes under this shared Resource handle.
	switch p := prov.(type) {
	case *provider_spacewave.Provider:
		sw := NewSpacewaveProviderResource(provResource, le, b, p)
		registrations = append(registrations, func(mux srpc.Mux) error {
			return s4wave_provider_spacewave.SRPCRegisterSpacewaveProviderResourceService(mux, sw)
		})
	case *provider_local.Provider:
		local := NewLocalProviderResource(provResource, le, b, p)
		provResource.release = local.Release
		registrations = append(registrations, func(mux srpc.Mux) error {
			return s4wave_provider_local.SRPCRegisterLocalProviderResourceService(mux, local)
		})
	}

	// Expose the complete method set after lifecycle wiring is installed.
	provResource.mux = resource_server.NewResourceMux(registrations...)
	return provResource
}

// Release drops provider-specific mounts. Repeated calls are safe.
func (r *ProviderResource) Release() {
	if r.release != nil {
		r.release()
	}
}

// GetMux returns the RPC mux.
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
	// Resolve the caller's Resource generation before mounting an account.
	resourceCtx, err := resource_server.MustGetResourceClientContext(ctx)
	if err != nil {
		return nil, err
	}

	// Retain the provider account until its child Resource is released.
	account, relFn, err := r.provider.AccessProviderAccount(ctx, req.GetAccountId(), nil)
	if err != nil {
		return nil, err
	}

	// Release the account wrapper before releasing the underlying account mount.
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

	// Return only a successfully registered child Resource.
	return &s4wave_provider.AccessProviderAccountResponse{ResourceId: id}, nil
}

// _ verifies the provider Resource contract.
var _ s4wave_provider.SRPCProviderResourceServiceServer = (*ProviderResource)(nil)
