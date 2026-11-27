package provider_spacewave

import (
	"context"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// SSOCodeExchange exchanges an authorization code using the account's entity client.
func (a *ProviderAccount) SSOCodeExchange(ctx context.Context, provider, code, redirectURI string) (*api.SSOCodeExchangeResponse, error) {
	return a.entityCli.SSOCodeExchange(ctx, provider, code, redirectURI)
}

// RegisterSessionWithRequest registers an entity-authorized session for the account.
func (a *ProviderAccount) RegisterSessionWithRequest(ctx context.Context, req *api.RegisterSessionRequest, turnstileToken string) (*api.RegisterSessionResponse, error) {
	return a.entityCli.RegisterSessionWithRequest(ctx, req, turnstileToken)
}

// RollbackSessionRegistration removes an incomplete entity-authorized registration.
func (a *ProviderAccount) RollbackSessionRegistration(ctx context.Context, sessionPeerID string) error {
	return a.entityCli.RollbackSessionRegistration(ctx, sessionPeerID)
}
