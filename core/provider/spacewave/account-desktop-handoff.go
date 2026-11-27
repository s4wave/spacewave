//go:build !js

package provider_spacewave

import (
	"context"

	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
)

// WaitForDesktopPasskeyRegister collects registration artifacts through the account's provider.
func (a *ProviderAccount) WaitForDesktopPasskeyRegister(ctx context.Context, nonce, wsTicket, openURL string) (*api.DesktopPasskeyRegisterResult, error) {
	return provider_spacewave_handoff.WaitForDesktopPasskeyRegister(ctx, a.p.GetHTTPClient(), a.p.GetEndpoint(), a.p.GetAccountEndpoint(), nonce, wsTicket, openURL)
}

// WaitForDesktopPasskeyReauth collects reauthentication artifacts through the account's provider.
func (a *ProviderAccount) WaitForDesktopPasskeyReauth(ctx context.Context, nonce, wsTicket, openURL string) (*api.DesktopPasskeyReauthResult, error) {
	return provider_spacewave_handoff.WaitForDesktopPasskeyReauth(ctx, a.p.GetHTTPClient(), a.p.GetEndpoint(), a.p.GetAccountEndpoint(), nonce, wsTicket, openURL)
}

// WaitForDesktopSSOLink collects an OAuth result through the account's provider.
func (a *ProviderAccount) WaitForDesktopSSOLink(ctx context.Context, provider, wsTicket, openURL string) (*api.DesktopSSOLinkResult, error) {
	return provider_spacewave_handoff.WaitForDesktopSSOLink(ctx, a.p.GetHTTPClient(), a.p.GetEndpoint(), provider, wsTicket, openURL)
}
