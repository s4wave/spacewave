package s4wave_provider_spacewave

import (
	"context"

	resource_client "github.com/s4wave/spacewave/bldr/resource/client"
)

// SpacewaveProvider is the SDK wrapper for the SpacewaveProviderResourceService.
type SpacewaveProvider struct {
	client  *resource_client.Client
	ref     resource_client.ResourceRef
	service SRPCSpacewaveProviderResourceServiceClient
}

// NewSpacewaveProvider creates a new SpacewaveProvider resource wrapper.
func NewSpacewaveProvider(client *resource_client.Client, ref resource_client.ResourceRef) (*SpacewaveProvider, error) {
	srpcClient, err := ref.GetClient()
	if err != nil {
		return nil, err
	}
	return &SpacewaveProvider{
		client:  client,
		ref:     ref,
		service: NewSRPCSpacewaveProviderResourceServiceClient(srpcClient),
	}, nil
}

// GetResourceRef returns the resource reference.
func (s *SpacewaveProvider) GetResourceRef() resource_client.ResourceRef {
	return s.ref
}

// Release releases the resource reference.
func (s *SpacewaveProvider) Release() {
	s.ref.Release()
}

// CreateAccount creates an account on the spacewave provider.
func (s *SpacewaveProvider) CreateAccount(ctx context.Context, entityID, password, turnstileToken string) (*CreateAccountResponse, error) {
	return s.service.CreateAccount(ctx, &CreateAccountRequest{
		EntityId:       entityID,
		TurnstileToken: turnstileToken,
		Credential: &CreateAccountRequest_Password{
			Password: &PasswordCredential{Password: password},
		},
	})
}

// LoginOrCreateAccount logs in or creates an account on the spacewave provider.
func (s *SpacewaveProvider) LoginOrCreateAccount(ctx context.Context, username, password string) (*LoginOrCreateAccountResponse, error) {
	return s.service.LoginOrCreateAccount(ctx, &LoginOrCreateAccountRequest{
		Username: username,
		Password: password,
	})
}

// LoginWithEntityKey creates a session using a pre-resolved entity private key.
func (s *SpacewaveProvider) LoginWithEntityKey(ctx context.Context, pemPrivateKey []byte) (*LoginWithEntityKeyResponse, error) {
	return s.service.LoginWithEntityKey(ctx, &LoginWithEntityKeyRequest{
		PemPrivateKey: pemPrivateKey,
	})
}

// PasskeyCheckUsername acknowledges the opaque first passkey step.
func (s *SpacewaveProvider) PasskeyCheckUsername(ctx context.Context, username string) (*PasskeyCheckUsernameResponse, error) {
	return s.service.PasskeyCheckUsername(ctx, &PasskeyCheckUsernameRequest{
		Username: username,
	})
}

// PasskeyRegisterChallenge fetches WebAuthn registration options for signup.
func (s *SpacewaveProvider) PasskeyRegisterChallenge(ctx context.Context, username string) (*PasskeyRegisterChallengeResponse, error) {
	return s.service.PasskeyRegisterChallenge(ctx, &PasskeyRegisterChallengeRequest{
		Username: username,
	})
}

// PasskeyAuthOptions fetches WebAuthn authentication options from the cloud.
func (s *SpacewaveProvider) PasskeyAuthOptions(ctx context.Context, username string) (*PasskeyAuthOptionsResponse, error) {
	return s.service.PasskeyAuthOptions(ctx, &PasskeyAuthOptionsRequest{
		Username: username,
	})
}

// PasskeyAuthVerify verifies a WebAuthn authentication credential with the cloud.
func (s *SpacewaveProvider) PasskeyAuthVerify(ctx context.Context, credentialJSON string) (*PasskeyAuthVerifyResponse, error) {
	return s.service.PasskeyAuthVerify(ctx, &PasskeyAuthVerifyRequest{
		CredentialJson: credentialJSON,
	})
}

// PasskeyConfirmSignup confirms browser-owned passkey signup for the web flow.
func (s *SpacewaveProvider) PasskeyConfirmSignup(ctx context.Context, req *PasskeyConfirmSignupRequest) (*PasskeyConfirmSignupResponse, error) {
	return s.service.PasskeyConfirmSignup(ctx, req)
}

// RelayDesktopPasskey relays a browser ceremony result back to native alpha.
func (s *SpacewaveProvider) RelayDesktopPasskey(ctx context.Context, req *RelayDesktopPasskeyRequest) (*RelayDesktopPasskeyResponse, error) {
	return s.service.RelayDesktopPasskey(ctx, req)
}

// RequestRecoveryEmail requests a recovery email for an account.
func (s *SpacewaveProvider) RequestRecoveryEmail(ctx context.Context, email, turnstileToken string) (*RequestRecoveryEmailResponse, error) {
	return s.service.RequestRecoveryEmail(ctx, &RequestRecoveryEmailRequest{
		Email:          email,
		TurnstileToken: turnstileToken,
	})
}

// RecoverVerify verifies a recovery token from an email link.
func (s *SpacewaveProvider) RecoverVerify(ctx context.Context, token string) (*RecoverVerifyResponse, error) {
	return s.service.RecoverVerify(ctx, &RecoverVerifyRequest{
		Token: token,
	})
}

// RecoverExecute completes account recovery.
func (s *SpacewaveProvider) RecoverExecute(ctx context.Context, req *RecoverExecuteRequest) (*RecoverExecuteResponse, error) {
	return s.service.RecoverExecute(ctx, req)
}

// GetLinkedCloudSession returns the session index of the linked cloud session.
func (s *SpacewaveProvider) GetLinkedCloudSession(ctx context.Context) (*GetLinkedCloudSessionResponse, error) {
	return s.service.GetLinkedCloudSession(ctx, &GetLinkedCloudSessionRequest{})
}

// StartBrowserHandoff opens the browser auth handoff flow on native clients.
func (s *SpacewaveProvider) StartBrowserHandoff(ctx context.Context, req *StartBrowserHandoffRequest) (*StartBrowserHandoffResponse, error) {
	return s.service.StartBrowserHandoff(ctx, req)
}

// StartDesktopSSO starts the native desktop SSO flow.
func (s *SpacewaveProvider) StartDesktopSSO(ctx context.Context, ssoProvider string) (*StartDesktopSSOResponse, error) {
	return s.service.StartDesktopSSO(ctx, &StartDesktopSSORequest{
		SsoProvider: ssoProvider,
	})
}

// ConfirmDesktopSSO completes native desktop SSO account creation.
func (s *SpacewaveProvider) ConfirmDesktopSSO(ctx context.Context, req *ConfirmDesktopSSORequest) (*ConfirmDesktopSSOResponse, error) {
	return s.service.ConfirmDesktopSSO(ctx, req)
}

// StartDesktopPasskey starts the native desktop passkey flow.
func (s *SpacewaveProvider) StartDesktopPasskey(ctx context.Context) (*StartDesktopPasskeyResponse, error) {
	return s.service.StartDesktopPasskey(ctx, &StartDesktopPasskeyRequest{})
}

// ConfirmDesktopPasskey completes native desktop passkey account creation.
func (s *SpacewaveProvider) ConfirmDesktopPasskey(ctx context.Context, req *ConfirmDesktopPasskeyRequest) (*ConfirmDesktopPasskeyResponse, error) {
	return s.service.ConfirmDesktopPasskey(ctx, req)
}
