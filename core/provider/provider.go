package provider

import (
	"context"

	"github.com/aperturerobotics/controllerbus/controller"
)

// Provider is the interface implemented by the provider controller.
type Provider interface {
	// GetProviderInfo returns the basic provider information.
	GetProviderInfo() *ProviderInfo

	// AccessProviderAccount accesses a provider account.
	// If accountID is empty, it will use the default or prompt the user.
	// Released may be nil.
	AccessProviderAccount(
		ctx context.Context,
		accountID string,
		released func(),
	) (ProviderAccount, func(), error)

	// Execute executes the provider.
	// Return nil for no-op (will not be restarted).
	Execute(ctx context.Context) error
}

// ProviderHandler manages a Provider and receives event callbacks.
// This is typically fulfilled by the provider controller.
type ProviderHandler any

// ProviderController is implemented by provider controllers.
type ProviderController interface {
	// Controller indicates this is a controller.
	controller.Controller

	// GetProviderInfo returns the basic provider information.
	GetProviderInfo() *ProviderInfo

	// GetProvider returns the provider, waiting for it to be ready.
	//
	// Returns nil, context.Canceled if canceled.
	GetProvider(ctx context.Context) (Provider, error)
}

// ProviderAccount represents an account for a provider.
type ProviderAccount interface {
	// GetProviderAccountFeature returns the implementation of a specific provider feature.
	//
	// Implements one of SpaceProvider, BlockStoreProvider, ...
	// Check GetProviderInfo()=>features in advance before calling this.
	// Returns ErrProviderFeatureUnimplemented if the feature is not implemented.
	GetProviderAccountFeature(ctx context.Context, feature ProviderFeature) (ProviderAccountFeature, error)
}

// GetProviderAccountFeature type asserts the provider account feature.
//
// Implements one of SpaceProvider, BlockStoreProvider, ...
// Check GetProviderInfo()=>features in advance before calling this.
// Returns ErrUnimplementedProviderFeature if the feature is not implemented.
func GetProviderAccountFeature[V ProviderAccountFeature](ctx context.Context, acc ProviderAccount, feature ProviderFeature) (V, error) {
	v, err := acc.GetProviderAccountFeature(ctx, feature)
	if err != nil {
		var empty V
		return empty, err
	}

	vv, ok := v.(V)
	if !ok {
		var empty V
		return empty, ErrUnimplementedProviderFeature
	}

	return vv, nil
}

// ProviderAccountFeature is a ProviderFeature implementation base type.
type ProviderAccountFeature any
