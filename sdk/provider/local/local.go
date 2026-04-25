package s4wave_provider_local

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

// LocalProvider is the SDK wrapper for the LocalProviderResourceService.
type LocalProvider struct {
	client  *resource_client.Client
	ref     resource_client.ResourceRef
	service SRPCLocalProviderResourceServiceClient
}

// NewLocalProvider creates a new LocalProvider resource wrapper.
func NewLocalProvider(client *resource_client.Client, ref resource_client.ResourceRef) (*LocalProvider, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &LocalProvider{
		client:  client,
		ref:     ref,
		service: NewSRPCLocalProviderResourceServiceClient(srpcClient),
	}, nil
}

// GetResourceRef returns the resource reference.
func (l *LocalProvider) GetResourceRef() resource_client.ResourceRef {
	return l.ref
}

// Release releases the resource reference.
func (l *LocalProvider) Release() {
	l.ref.Release()
}

// CreateAccount creates a ProviderAccount and Session on the local provider.
func (l *LocalProvider) CreateAccount(ctx context.Context) (*CreateAccountResponse, error) {
	return l.service.CreateAccount(ctx, &CreateAccountRequest{})
}
