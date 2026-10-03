package resource_provider

import (
	"context"
	"crypto/ecdh"
	crand "crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/scrub"
	"github.com/pkg/errors"
	auth_method_password "github.com/s4wave/spacewave/auth/method/password"
	"github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/resource/provider/entitykeylogin"
	"github.com/s4wave/spacewave/core/session"
	session_lock "github.com/s4wave/spacewave/core/session/lock"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

// recoveryContext is the signing context for account recovery.
const recoveryContext = "spacewave 2026-03-19 account recovery v1."

const (
	passkeyPrfSaltSize = 32
)

// SpacewaveProviderResource implements the SpacewaveProviderResourceService.
type SpacewaveProviderResource struct {
	*ProviderResource
	le       *logrus.Entry
	b        bus.Bus
	provider *provider_spacewave.Provider
}

// NewSpacewaveProviderResource creates a new SpacewaveProviderResource.
func NewSpacewaveProviderResource(pr *ProviderResource, le *logrus.Entry, b bus.Bus, prov *provider_spacewave.Provider) *SpacewaveProviderResource {
	return &SpacewaveProviderResource{
		ProviderResource: pr,
		le:               le,
		b:                b,
		provider:         prov,
	}
}

// CreateAccount creates an account on the spacewave provider.
func (s *SpacewaveProviderResource) CreateAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.CreateAccountRequest,
) (*s4wave_provider_spacewave.CreateAccountResponse, error) {
	// Require the entity ID for cloud account creation.
	entityID := req.GetEntityId()
	if entityID == "" {
		return nil, errors.New("entity_id is required")
	}
	turnstileToken := req.GetTurnstileToken()

	// Extract password from credential oneof.
	var password string
	switch cred := req.GetCredential().(type) {
	case *s4wave_provider_spacewave.CreateAccountRequest_Password:
		password = cred.Password.GetPassword()
		if password == "" {
			return nil, errors.New("password is required")
		}
	case *s4wave_provider_spacewave.CreateAccountRequest_Pem:
		return nil, errors.New("pem account creation not yet implemented")
	case *s4wave_provider_spacewave.CreateAccountRequest_Passkey:
		return nil, errors.New("passkey account creation not yet implemented")
	default:
		return nil, errors.New("credential is required")
	}

	// Retain the Session controller while creating the cloud account.
	s.le.WithField("entity-id-len", len(entityID)).Debug("creating spacewave account: lookup session controller")
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Create the cloud account and register its Session.
	s.le.WithField("entity-id-len", len(entityID)).Debug("creating spacewave account: register cloud account")
	listEntry, err := s.provider.CreateSpacewaveAccountAndSession(ctx, entityID, []byte(password), turnstileToken, sessionCtrl)
	if err != nil {
		return nil, err
	}
	s.le.WithField("entity-id-len", len(entityID)).Debug("creating spacewave account: session ready")

	return &s4wave_provider_spacewave.CreateAccountResponse{SessionListEntry: listEntry}, nil
}

// LoginAccount attempts to log in to an existing account without creating one.
// Returns a result oneof: session on success, is_new_account if the account
// does not exist, or error_code if the credentials are wrong.
func (s *SpacewaveProviderResource) LoginAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.LoginAccountRequest,
) (*s4wave_provider_spacewave.LoginAccountResponse, error) {
	// Require the entity ID for cloud account login.
	entityID := req.GetEntityId()
	if entityID == "" {
		return nil, errors.New("entity_id is required")
	}
	turnstileToken := req.GetTurnstileToken()

	// Derive keypair based on credential type.
	var privKey crypto.PrivKey
	switch cred := req.GetCredential().(type) {
	case *s4wave_provider_spacewave.LoginAccountRequest_Password:
		password := cred.Password.GetPassword()
		if password == "" {
			return nil, errors.New("password is required")
		}
		var err error
		_, privKey, err = auth_method_password.BuildParametersWithUsernamePassword(entityID, []byte(password))
		if err != nil {
			return nil, errors.Wrap(err, "derive entity keypair")
		}
	case *s4wave_provider_spacewave.LoginAccountRequest_Pem:
		return nil, errors.New("pem login not yet implemented")
	case *s4wave_provider_spacewave.LoginAccountRequest_Passkey:
		return nil, errors.New("passkey login not yet implemented")
	default:
		return nil, errors.New("credential is required")
	}

	// Derive the entity identity used to sign the login request.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive peer id")
	}

	// Retain the Session controller while registering the account login.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Log in with the entity key and translate cloud credential failures.
	entityCli := provider_spacewave.NewEntityClientDirect(
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		s.provider.GetSigningEnvPrefix(),
		privKey,
		peerID,
	)
	listEntry, loginErr := s.provider.LoginExistingAccount(ctx, entityCli, privKey, peerID, entityID, turnstileToken, sessionCtrl)
	if loginErr != nil {
		if errors.Is(loginErr, provider_spacewave.ErrUnknownEntity) {
			return &s4wave_provider_spacewave.LoginAccountResponse{
				Result: &s4wave_provider_spacewave.LoginAccountResponse_ErrorCode{
					ErrorCode: "wrong_password",
				},
			}, nil
		}
		if errors.Is(loginErr, provider_spacewave.ErrUnknownKeypair) {
			return &s4wave_provider_spacewave.LoginAccountResponse{
				Result: &s4wave_provider_spacewave.LoginAccountResponse_IsNewAccount{
					IsNewAccount: true,
				},
			}, nil
		}
		return nil, loginErr
	}

	return &s4wave_provider_spacewave.LoginAccountResponse{
		Result: &s4wave_provider_spacewave.LoginAccountResponse_Session{
			Session: listEntry,
		},
	}, nil
}

// LoginOrCreateAccount logs in or creates an account on the spacewave provider.
func (s *SpacewaveProviderResource) LoginOrCreateAccount(
	ctx context.Context,
	req *s4wave_provider_spacewave.LoginOrCreateAccountRequest,
) (*s4wave_provider_spacewave.LoginOrCreateAccountResponse, error) {
	// Require the username and password for account login or creation.
	username := req.GetUsername()
	if username == "" {
		return nil, errors.New("username is required")
	}
	password := req.GetPassword()
	if password == "" {
		return nil, errors.New("password is required")
	}

	// Retain the Session controller while opening the cloud account.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Log in or create the cloud account and its Session.
	listEntry, isNew, err := s.provider.LoginOrCreateAccount(ctx, username, []byte(password), sessionCtrl)
	if err != nil {
		return nil, err
	}

	return &s4wave_provider_spacewave.LoginOrCreateAccountResponse{
		SessionListEntry: listEntry,
		IsNewAccount:     isNew,
	}, nil
}

// LoginWithEntityKey creates a session using a pre-resolved entity private key.
func (s *SpacewaveProviderResource) LoginWithEntityKey(
	ctx context.Context,
	req *s4wave_provider_spacewave.LoginWithEntityKeyRequest,
) (*s4wave_provider_spacewave.LoginWithEntityKeyResponse, error) {
	// Parse the entity key and derive its signing identity.
	privKey, err := entitykeylogin.ParsePrivateKey(req.GetPemPrivateKey())
	if err != nil {
		return nil, err
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive peer ID from entity key")
	}

	// Retain the Session controller while logging in with the entity key.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Authenticate the entity key and register its cloud Session.
	entityCli := provider_spacewave.NewEntityClientDirect(
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		s.provider.GetSigningEnvPrefix(),
		privKey,
		peerID,
	)
	listEntry, err := s.provider.LoginExistingAccount(ctx, entityCli, privKey, peerID, "", "", sessionCtrl)
	if err != nil {
		return nil, errors.Wrap(err, "login with entity key")
	}

	return &s4wave_provider_spacewave.LoginWithEntityKeyResponse{
		SessionListEntry: listEntry,
	}, nil
}

// MountLinkedDeviceSession mounts a SpaceLink-approved DEVICE session.
func (s *SpacewaveProviderResource) MountLinkedDeviceSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.MountLinkedDeviceSessionRequest,
) (*s4wave_provider_spacewave.MountLinkedDeviceSessionResponse, error) {
	// Require the linked account, Session ID, and private key.
	if req.GetAccountId() == "" {
		return nil, errors.New("account_id is required")
	}
	if err := provider.ValidateResourceID(req.GetSessionId()); err != nil {
		return nil, errors.Wrap(err, "session_id")
	}
	if len(req.GetSessionPemPrivateKey()) == 0 {
		return nil, errors.New("session_pem_private_key is required")
	}

	// Parse the linked Session key and verify its peer identity.
	privKey, err := keypem.ParsePrivKeyPem(req.GetSessionPemPrivateKey())
	if err != nil {
		return nil, errors.Wrap(err, "parse session private key")
	}
	if privKey == nil {
		return nil, errors.New("session_pem_private_key must contain a private key")
	}
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive session peer id")
	}
	if req.GetSessionPeerId() != "" && req.GetSessionPeerId() != peerID.String() {
		return nil, errors.New("session_peer_id does not match session private key")
	}

	// Retain the Session controller while mounting the linked device.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, errors.Wrap(err, "lookup session controller")
	}
	defer sessionCtrlRef.Release()

	// Mount the approved device Session on the cloud account.
	listEntry, err := s.provider.MountLinkedDeviceSession(
		ctx,
		req.GetAccountId(),
		req.GetSessionId(),
		req.GetLabel(),
		privKey,
		sessionCtrl,
	)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.MountLinkedDeviceSessionResponse{
		SessionListEntry: listEntry,
	}, nil
}

// GenerateAuthKeypairs generates account and session auth key material.
func (s *SpacewaveProviderResource) GenerateAuthKeypairs(
	context.Context,
	*s4wave_provider_spacewave.GenerateAuthKeypairsRequest,
) (*s4wave_provider_spacewave.GenerateAuthKeypairsResponse, error) {
	// Generate the entity signing identity and encode its private key.
	entityPriv, _, err := crypto.GenerateEd25519Key(crand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "generate entity keypair")
	}
	entityPeerID, err := peer.IDFromPrivateKey(entityPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive entity peer id")
	}
	entityPEM, err := keypem.MarshalPrivKeyPem(entityPriv)
	if err != nil {
		return nil, errors.Wrap(err, "marshal entity keypair PEM")
	}
	defer scrub.Scrub(entityPEM)

	// Generate the Session signing identity for account authentication.
	sessionPriv, _, err := crypto.GenerateEd25519Key(crand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "generate session keypair")
	}
	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive session peer id")
	}

	// Encode the entity key for the authentication response.
	pem := string(entityPEM)
	return &s4wave_provider_spacewave.GenerateAuthKeypairsResponse{
		Entity: &s4wave_provider_spacewave.GeneratedEntityKeypair{
			PemPrivateKey:      pem,
			PeerId:             entityPeerID.String(),
			CustodiedPemBase64: base64.StdEncoding.EncodeToString([]byte(pem)),
		},
		Session: &s4wave_provider_spacewave.GeneratedSessionKeypair{
			PeerId: sessionPeerID.String(),
		},
	}, nil
}

// WrapPemWithPin wraps an entity PEM with age scrypt PIN encryption.
func (s *SpacewaveProviderResource) WrapPemWithPin(
	ctx context.Context,
	req *s4wave_provider_spacewave.WrapPemWithPinRequest,
) (*s4wave_provider_spacewave.WrapPemWithPinResponse, error) {
	// Require the PEM and the PIN.
	pem := req.GetPemPrivateKey()
	if pem == "" {
		return nil, errors.New("pem_private_key is required")
	}
	pin := req.GetPin()
	if pin == "" {
		return nil, errors.New("pin is required")
	}

	// Wrap the PEM and encode the result.
	wrapped, err := provider_spacewave.WrapPemWithPin([]byte(pem), pin)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.WrapPemWithPinResponse{
		WrappedPemBase64: base64.StdEncoding.EncodeToString(wrapped),
	}, nil
}

// UnwrapPemWithPin unwraps an entity PEM with age scrypt PIN encryption.
func (s *SpacewaveProviderResource) UnwrapPemWithPin(
	ctx context.Context,
	req *s4wave_provider_spacewave.UnwrapPemWithPinRequest,
) (*s4wave_provider_spacewave.UnwrapPemWithPinResponse, error) {
	// Require the wrapped PEM and the PIN.
	wrapped := req.GetWrappedPemBase64()
	if wrapped == "" {
		return nil, errors.New("wrapped_pem_base64 is required")
	}
	pin := req.GetPin()
	if pin == "" {
		return nil, errors.New("pin is required")
	}

	// Decode and unwrap the PEM.
	encrypted, err := base64.StdEncoding.DecodeString(wrapped)
	if err != nil {
		return nil, errors.Wrap(err, "decode wrapped PEM")
	}
	pem, err := provider_spacewave.UnwrapPemWithPin(encrypted, pin)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.UnwrapPemWithPinResponse{
		PemPrivateKey: pem,
	}, nil
}

// GeneratePasskeyPrfSalt creates a WebAuthn PRF salt.
func (s *SpacewaveProviderResource) GeneratePasskeyPrfSalt(
	ctx context.Context,
	_ *s4wave_provider_spacewave.GeneratePasskeyPrfSaltRequest,
) (*s4wave_provider_spacewave.GeneratePasskeyPrfSaltResponse, error) {
	salt := make([]byte, passkeyPrfSaltSize)
	if _, err := io.ReadFull(crand.Reader, salt); err != nil {
		return nil, errors.Wrap(err, "generate passkey PRF salt")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.GeneratePasskeyPrfSaltResponse{
		PrfSalt: base64.RawURLEncoding.EncodeToString(salt),
	}, nil
}

// WrapWithPasskeyPrf wraps an auth blob with the WebAuthn PRF output.
func (s *SpacewaveProviderResource) WrapWithPasskeyPrf(
	ctx context.Context,
	req *s4wave_provider_spacewave.WrapWithPasskeyPrfRequest,
) (*s4wave_provider_spacewave.WrapWithPasskeyPrfResponse, error) {
	// Require the plaintext.
	plaintext := req.GetPlaintext()
	if len(plaintext) == 0 {
		return nil, errors.New("plaintext is required")
	}

	// Seal the plaintext.
	ciphertext, params, err := provider_spacewave.WrapWithPasskeyPrf(plaintext, req.GetPrfOutput(), req.GetPinWrapped())
	if err != nil {
		return nil, err
	}

	// Encode the blob and its parameters.
	authParams, err := params.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal passkey PRF auth params")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.WrapWithPasskeyPrfResponse{
		EncryptedBlobBase64: base64.StdEncoding.EncodeToString(ciphertext),
		AuthParamsBase64:    base64.StdEncoding.EncodeToString(authParams),
	}, nil
}

// UnwrapWithPasskeyPrf unwraps an auth blob with the WebAuthn PRF output.
func (s *SpacewaveProviderResource) UnwrapWithPasskeyPrf(
	ctx context.Context,
	req *s4wave_provider_spacewave.UnwrapWithPasskeyPrfRequest,
) (*s4wave_provider_spacewave.UnwrapWithPasskeyPrfResponse, error) {
	// Require the blob and its parameters.
	encryptedBlob := req.GetEncryptedBlobBase64()
	if encryptedBlob == "" {
		return nil, errors.New("encrypted_blob_base64 is required")
	}
	authParams := req.GetAuthParamsBase64()
	if authParams == "" {
		return nil, errors.New("auth_params_base64 is required")
	}

	// Decode the blob and its parameters.
	ciphertext, err := base64.StdEncoding.DecodeString(encryptedBlob)
	if err != nil {
		return nil, errors.Wrap(err, "decode passkey PRF encrypted blob")
	}
	authParamsBytes, err := base64.StdEncoding.DecodeString(authParams)
	if err != nil {
		return nil, errors.Wrap(err, "decode passkey PRF auth params")
	}
	params := &s4wave_provider_spacewave.PasskeyPrfAuthParams{}
	if err := params.UnmarshalVT(authParamsBytes); err != nil {
		return nil, errors.Wrap(err, "unmarshal passkey PRF auth params")
	}

	// Open the blob with the PRF output.
	plaintext, err := provider_spacewave.UnwrapWithPasskeyPrf(ciphertext, params, req.GetPrfOutput())
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.UnwrapWithPasskeyPrfResponse{
		Plaintext:  plaintext,
		PinWrapped: params.GetPinWrapped(),
	}, nil
}

// PasskeyCheckUsername acknowledges the opaque first passkey step.
func (s *SpacewaveProviderResource) PasskeyCheckUsername(
	ctx context.Context,
	req *s4wave_provider_spacewave.PasskeyCheckUsernameRequest,
) (*s4wave_provider_spacewave.PasskeyCheckUsernameResponse, error) {
	ok, err := provider_spacewave.PasskeyCheckUsername(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		req.GetUsername(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "passkey check username")
	}
	return &s4wave_provider_spacewave.PasskeyCheckUsernameResponse{
		Ok: ok,
	}, nil
}

// PasskeyRegisterChallenge fetches WebAuthn registration options for signup.
func (s *SpacewaveProviderResource) PasskeyRegisterChallenge(
	ctx context.Context,
	req *s4wave_provider_spacewave.PasskeyRegisterChallengeRequest,
) (*s4wave_provider_spacewave.PasskeyRegisterChallengeResponse, error) {
	optionsJSON, err := provider_spacewave.PasskeyRegisterChallenge(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		req.GetUsername(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "passkey register challenge")
	}
	return &s4wave_provider_spacewave.PasskeyRegisterChallengeResponse{
		OptionsJson: optionsJSON,
	}, nil
}

// PasskeyAuthOptions fetches WebAuthn authentication options from the cloud.
func (s *SpacewaveProviderResource) PasskeyAuthOptions(
	ctx context.Context,
	req *s4wave_provider_spacewave.PasskeyAuthOptionsRequest,
) (*s4wave_provider_spacewave.PasskeyAuthOptionsResponse, error) {
	optionsJSON, err := provider_spacewave.PasskeyAuthOptions(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		req.GetUsername(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "passkey auth options")
	}
	return &s4wave_provider_spacewave.PasskeyAuthOptionsResponse{
		OptionsJson: optionsJSON,
	}, nil
}

// PasskeyAuthVerify verifies a WebAuthn authentication credential with the cloud.
func (s *SpacewaveProviderResource) PasskeyAuthVerify(
	ctx context.Context,
	req *s4wave_provider_spacewave.PasskeyAuthVerifyRequest,
) (*s4wave_provider_spacewave.PasskeyAuthVerifyResponse, error) {
	result, err := provider_spacewave.PasskeyAuthVerify(ctx, s.provider.GetHTTPClient(), s.provider.GetEndpoint(), req.GetCredentialJson())
	if err != nil {
		return nil, errors.Wrap(err, "passkey auth verify")
	}

	return &s4wave_provider_spacewave.PasskeyAuthVerifyResponse{
		AccountId:     result.GetAccountId(),
		EntityId:      result.GetEntityId(),
		EncryptedBlob: result.GetEncryptedBlob(),
		PrfCapable:    result.GetPrfCapable(),
		PrfSalt:       result.GetPrfSalt(),
		AuthParams:    result.GetAuthParams(),
		PinWrapped:    result.GetPinWrapped(),
	}, nil
}

// PasskeyConfirmSignup confirms browser-owned passkey signup for the web flow.
func (s *SpacewaveProviderResource) PasskeyConfirmSignup(
	ctx context.Context,
	req *s4wave_provider_spacewave.PasskeyConfirmSignupRequest,
) (*s4wave_provider_spacewave.PasskeyConfirmSignupResponse, error) {
	err := provider_spacewave.ConfirmPasskeySignup(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		&provider_spacewave.ConfirmPasskeySignupRequest{
			CredentialJSON:   req.GetCredentialJson(),
			Username:         req.GetUsername(),
			WrappedEntityKey: req.GetWrappedEntityKey(),
			EntityPeerID:     req.GetEntityPeerId(),
			SessionPeerID:    req.GetSessionPeerId(),
			PinWrapped:       req.GetPinWrapped(),
			PrfCapable:       req.GetPrfCapable(),
			PrfSalt:          req.GetPrfSalt(),
			AuthParams:       req.GetAuthParams(),
		},
	)
	if err != nil {
		return nil, errors.Wrap(err, "confirm passkey signup")
	}
	return &s4wave_provider_spacewave.PasskeyConfirmSignupResponse{
		SessionPeerId: req.GetSessionPeerId(),
	}, nil
}

// RelayDesktopPasskey relays a browser ceremony result back to native alpha.
func (s *SpacewaveProviderResource) RelayDesktopPasskey(
	ctx context.Context,
	req *s4wave_provider_spacewave.RelayDesktopPasskeyRequest,
) (*s4wave_provider_spacewave.RelayDesktopPasskeyResponse, error) {
	// Translate the browser passkey result and relay it to the desktop.
	relayReq := &api.DesktopPasskeyRelayResult{
		Nonce: req.GetNonce(),
	}
	if linked := req.GetLinked(); linked != nil {
		relayReq.Result = &api.DesktopPasskeyRelayResult_Linked{
			Linked: &api.DesktopPasskeyLinkedResult{
				EncryptedBlob: linked.GetEncryptedBlob(),
				PrfCapable:    linked.GetPrfCapable(),
				PrfSalt:       linked.GetPrfSalt(),
				AuthParams:    linked.GetAuthParams(),
				PinWrapped:    linked.GetPinWrapped(),
				PrfOutput:     linked.GetPrfOutput(),
			},
		}
	}
	if newAccount := req.GetNewAccount(); newAccount != nil {
		relayReq.Result = &api.DesktopPasskeyRelayResult_NewAccount{
			NewAccount: &api.DesktopPasskeyNewAccountResult{
				Username:       newAccount.GetUsername(),
				CredentialJson: newAccount.GetCredentialJson(),
				PrfCapable:     newAccount.GetPrfCapable(),
				PrfSalt:        newAccount.GetPrfSalt(),
				PrfOutput:      newAccount.GetPrfOutput(),
			},
		}
	}
	if err := provider_spacewave.RelayDesktopPasskey(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		relayReq,
	); err != nil {
		return nil, errors.Wrap(err, "relay desktop passkey")
	}
	return &s4wave_provider_spacewave.RelayDesktopPasskeyResponse{}, nil
}

// SSOCodeExchange exchanges an OAuth authorization code for account info.
func (s *SpacewaveProviderResource) SSOCodeExchange(
	ctx context.Context,
	req *s4wave_provider_spacewave.SSOCodeExchangeRequest,
) (*s4wave_provider_spacewave.SSOCodeExchangeResponse, error) {
	result, err := provider_spacewave.SSOCodeExchange(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		req.GetProvider(),
		req.GetCode(),
		req.GetRedirectUri(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "sso callback")
	}
	return &s4wave_provider_spacewave.SSOCodeExchangeResponse{
		Linked:        result.GetLinked(),
		AccountId:     result.GetAccountId(),
		EntityId:      result.GetEntityId(),
		EncryptedBlob: result.GetEncryptedBlob(),
		PinWrapped:    result.GetPinWrapped(),
		AuthParams:    result.GetAuthParams(),
		SsoProvider:   result.GetProvider(),
		Email:         result.GetEmail(),
		Username:      result.GetUsername(),
	}, nil
}

// PrepareBrowserSSO creates the verifier and ephemeral device key retained by one browser flow.
func (s *SpacewaveProviderResource) PrepareBrowserSSO(
	_ context.Context,
	_ *s4wave_provider_spacewave.PrepareBrowserSSORequest,
) (*s4wave_provider_spacewave.PrepareBrowserSSOResponse, error) {
	// Generate the verifier and ephemeral device key for browser SSO.
	verifier := make([]byte, 32)
	if _, err := crand.Read(verifier); err != nil {
		return nil, errors.Wrap(err, "generate browser sso verifier")
	}
	privateKey, err := ecdh.X25519().GenerateKey(crand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "generate browser sso device key")
	}
	commitment := sha256.Sum256(verifier)
	return &s4wave_provider_spacewave.PrepareBrowserSSOResponse{
		Verifier:         verifier,
		VerifierHash:     commitment[:],
		DevicePublicKey:  privateKey.PublicKey().Bytes(),
		DevicePrivateKey: privateKey.Bytes(),
	}, nil
}

// SSONonceExchange exchanges an auth-session nonce for the stored SSO result.
func (s *SpacewaveProviderResource) SSONonceExchange(
	ctx context.Context,
	req *s4wave_provider_spacewave.SSONonceExchangeRequest,
) (*s4wave_provider_spacewave.SSOCodeExchangeResponse, error) {
	result, err := provider_spacewave.SSONonceExchange(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		req.GetNonce(),
		req.GetVerifier(),
		req.GetDevicePrivateKey(),
	)
	if err != nil {
		return nil, errors.Wrap(err, "sso nonce exchange")
	}
	return &s4wave_provider_spacewave.SSOCodeExchangeResponse{
		Linked:        result.GetLinked(),
		AccountId:     result.GetAccountId(),
		EntityId:      result.GetEntityId(),
		EncryptedBlob: result.GetEncryptedBlob(),
		PinWrapped:    result.GetPinWrapped(),
		AuthParams:    result.GetAuthParams(),
		SsoProvider:   result.GetProvider(),
		Email:         result.GetEmail(),
		Username:      result.GetUsername(),
	}, nil
}

// GetCloudProviderConfig returns pre-auth provider configuration.
func (s *SpacewaveProviderResource) GetCloudProviderConfig(
	ctx context.Context,
	req *s4wave_provider_spacewave.GetCloudProviderConfigRequest,
) (*s4wave_provider_spacewave.CloudProviderConfig, error) {
	authConfig, release, err := s.provider.GetCloudConfig(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get cloud config")
	}
	defer release()
	return &s4wave_provider_spacewave.CloudProviderConfig{
		SsoBaseUrl:       authConfig.GetSsoBaseUrl(),
		ExchangeUrl:      authConfig.GetExchangeUrl(),
		ConfirmUrl:       authConfig.GetConfirmUrl(),
		TurnstileSiteKey: authConfig.GetTurnstileSiteKey(),
		AccountBaseUrl:   authConfig.GetAccountBaseUrl(),
		PublicBaseUrl:    authConfig.GetPublicBaseUrl(),
		GoogleSsoEnabled: authConfig.GetGoogleSsoEnabled(),
		GithubSsoEnabled: authConfig.GetGithubSsoEnabled(),
	}, nil
}

// RequestRecoveryEmail requests a recovery email from the cloud.
func (s *SpacewaveProviderResource) RequestRecoveryEmail(
	ctx context.Context,
	req *s4wave_provider_spacewave.RequestRecoveryEmailRequest,
) (*s4wave_provider_spacewave.RequestRecoveryEmailResponse, error) {
	// Require the recovery email address and request its cloud delivery.
	email := req.GetEmail()
	if email == "" {
		return nil, errors.New("email is required")
	}
	err := provider_spacewave.RequestRecoveryEmail(
		ctx,
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		email,
		req.GetTurnstileToken(),
	)
	if err != nil {
		return nil, err
	}
	return &s4wave_provider_spacewave.RequestRecoveryEmailResponse{
		Sent: true,
	}, nil
}

// RecoverVerify verifies a recovery token from an email link.
func (s *SpacewaveProviderResource) RecoverVerify(
	ctx context.Context,
	req *s4wave_provider_spacewave.RecoverVerifyRequest,
) (*s4wave_provider_spacewave.RecoverVerifyResponse, error) {
	// Require the email recovery token before cloud verification.
	token := req.GetToken()
	if token == "" {
		return nil, errors.New("token is required")
	}

	// Verify the recovery token and retrieve its account identity.
	result, err := provider_spacewave.RecoverVerify(ctx, s.provider.GetHTTPClient(), s.provider.GetEndpoint(), token)
	if err != nil {
		return nil, errors.Wrap(err, "recover verify")
	}

	return &s4wave_provider_spacewave.RecoverVerifyResponse{
		AccountId: result.GetAccountId(),
		EntityId:  result.GetEntityId(),
	}, nil
}

// RecoverExecute completes account recovery by deriving a new password
// keypair, signing the recovery request, and registering the new keypair.
func (s *SpacewaveProviderResource) RecoverExecute(
	ctx context.Context,
	req *s4wave_provider_spacewave.RecoverExecuteRequest,
) (*s4wave_provider_spacewave.RecoverExecuteResponse, error) {
	// Require the recovery token and replacement password credentials.
	token := req.GetToken()
	if token == "" {
		return nil, errors.New("token is required")
	}
	username := req.GetUsername()
	if username == "" {
		return nil, errors.New("username is required")
	}
	newPassword := req.GetNewPassword()
	if newPassword == "" {
		return nil, errors.New("new_password is required")
	}

	// Derive the replacement entity key from the recovery password.
	params, privKey, err := auth_method_password.BuildParametersWithUsernamePassword(username, []byte(newPassword))
	if err != nil {
		return nil, errors.Wrap(err, "derive entity keypair")
	}

	// Encode the replacement password authentication parameters.
	authParams, err := params.MarshalBlock()
	if err != nil {
		return nil, errors.Wrap(err, "marshal auth params")
	}

	// Derive the replacement entity identity for recovery signing.
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive peer id")
	}

	// Require the account ID bound into the recovery signature.
	accountID := req.GetAccountId()
	if accountID == "" {
		return nil, errors.New("account_id is required for recovery signing")
	}

	// Sign the recovery context, account, token, and replacement identity.
	peerIDStr := peerID.String()
	payload := make([]byte, 0, len(recoveryContext)+len(accountID)+len(token)+len(peerIDStr))
	payload = append(payload, recoveryContext...)
	payload = append(payload, accountID...)
	payload = append(payload, token...)
	payload = append(payload, peerIDStr...)
	sig, err := privKey.Sign(payload)
	if err != nil {
		return nil, errors.Wrap(err, "sign recovery message")
	}

	// Build the recovery request with the replacement key and signature.
	execReq := &api.RecoverExecuteRequest{
		Token: token,
		AddKeypair: &api.RecoverExecuteKeypair{
			PeerId:     peerIDStr,
			AuthMethod: auth_method_password.MethodID,
			AuthParams: base64.StdEncoding.EncodeToString(authParams),
		},
		Signatures: []*api.RecoverExecuteSignature{{
			PeerId:    peerIDStr,
			Signature: base64.StdEncoding.EncodeToString(sig),
		}},
		RemovePeerId: req.GetRemovePeerId(),
	}

	// Submit the signed recovery request to the cloud.
	if err := provider_spacewave.RecoverExecute(ctx, s.provider.GetHTTPClient(), s.provider.GetEndpoint(), execReq); err != nil {
		return nil, errors.Wrap(err, "recover execute")
	}

	return &s4wave_provider_spacewave.RecoverExecuteResponse{
		PeerId: peerIDStr,
	}, nil
}

// GetLinkedCloudSession returns the session index of the linked cloud session.
// Called from a local session context: reads the linked-cloud key from the
// local provider's ObjectStore, then walks the session list for a spacewave
// session with the matching provider account ID.
func (s *SpacewaveProviderResource) GetLinkedCloudSession(
	ctx context.Context,
	req *s4wave_provider_spacewave.GetLinkedCloudSessionRequest,
) (*s4wave_provider_spacewave.GetLinkedCloudSessionResponse, error) {
	// Retain the Session controller while finding the linked cloud Session.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Read the Session inventory used to match local and cloud accounts.
	sessions, err := sessionCtrl.ListSessions(ctx)
	if err != nil {
		return nil, err
	}

	// Find the active local session and read its linked-cloud account ID.
	var cloudAccountID string
	var localProviderID, localAccountID string
	for _, entry := range sessions {
		ref := entry.GetSessionRef()
		provRef := ref.GetProviderResourceRef()
		if provRef.GetProviderId() == "spacewave" {
			continue
		}

		// Read the linked-cloud key from the local provider's ObjectStore.
		providerID := provRef.GetProviderId()
		accountID := provRef.GetProviderAccountId()
		sessionID := provRef.GetId()

		volID := provider_local.StorageVolumeID(providerID, accountID)
		objectStoreID := provider_local.SessionObjectStoreID(providerID, accountID)
		objStoreHandle, _, diRef, oErr := volume.ExBuildObjectStoreAPI(ctx, s.b, false, objectStoreID, volID, nil)
		if oErr != nil {
			continue
		}
		objStore := objStoreHandle.GetObjectStore()
		var data []byte
		var found bool
		gErr := kvtx.RunTransaction(ctx, false,
			func(ctx context.Context) (kvtx.Tx, error) {
				return objStore.NewTransaction(ctx, false)
			},
			func(ctx context.Context, tx kvtx.Tx) error {
				var err error
				data, found, err = tx.Get(ctx, provider_local.LinkedCloudKey(sessionID))
				return err
			},
		)
		diRef.Release()
		if gErr != nil || !found {
			continue
		}
		cloudAccountID = string(data)
		localProviderID = providerID
		localAccountID = accountID
		break
	}

	// Return a missing result when no local Session links to a cloud account.
	if cloudAccountID == "" {
		return &s4wave_provider_spacewave.GetLinkedCloudSessionResponse{Found: false}, nil
	}

	// Check if the local session has any SharedObjects (empty = safe to skip migration).
	localEmpty := true
	if localProviderID != "" {
		volID := provider_local.StorageVolumeID(localProviderID, localAccountID)
		soObjStoreID := provider_local.SobjectObjectStoreID(localProviderID, localAccountID)
		soHandle, _, soDiRef, soErr := volume.ExBuildObjectStoreAPI(ctx, s.b, false, soObjStoreID, volID, nil)
		if soErr == nil {
			soStore := soHandle.GetObjectStore()
			var data []byte
			var found bool
			gErr := kvtx.RunTransaction(ctx, false,
				func(ctx context.Context) (kvtx.Tx, error) {
					return soStore.NewTransaction(ctx, false)
				},
				func(ctx context.Context, tx kvtx.Tx) error {
					var err error
					data, found, err = tx.Get(ctx, provider_local.SobjectObjectStoreListKey())
					return err
				},
			)
			if gErr == nil && found {
				list := &sobject.SharedObjectList{}
				if err := list.UnmarshalVT(data); err == nil && len(list.GetSharedObjects()) > 0 {
					localEmpty = false
				}
			}
			soDiRef.Release()
		}
	}

	// Walk session list for a spacewave session with the matching account ID.
	for _, entry := range sessions {
		provRef := entry.GetSessionRef().GetProviderResourceRef()
		if provRef.GetProviderId() != "spacewave" {
			continue
		}
		if provRef.GetProviderAccountId() == cloudAccountID {
			return &s4wave_provider_spacewave.GetLinkedCloudSessionResponse{
				Found:             true,
				SessionIndex:      entry.GetSessionIndex(),
				LocalSessionEmpty: localEmpty,
			}, nil
		}
	}

	return &s4wave_provider_spacewave.GetLinkedCloudSessionResponse{Found: false}, nil
}

// ReauthenticateSession re-authenticates a session whose key became stale.
// Derives an entity key from the credential, verifies with the cloud,
// generates a new session key, and clears the UNAUTHENTICATED status.
func (s *SpacewaveProviderResource) ReauthenticateSession(ctx context.Context, req *s4wave_provider_spacewave.ReauthenticateSessionRequest) (*s4wave_provider_spacewave.ReauthenticateSessionResponse, error) {
	// Require the Session index whose credentials need replacement.
	sessionIdx := req.GetSessionIndex()
	if sessionIdx == 0 {
		return nil, errors.New("session_index is required")
	}

	// Look up the session to get its provider account ID and session ref.
	sessionCtrl, sessionCtrlRef, err := session.ExLookupSessionController(ctx, s.b, "", false, nil)
	if err != nil {
		return nil, err
	}
	defer sessionCtrlRef.Release()

	// Find the registered Session that needs reauthentication.
	entry, err := sessionCtrl.GetSessionByIdx(ctx, sessionIdx)
	if err != nil {
		return nil, errors.Wrap(err, "get session by index")
	}
	if entry == nil {
		return nil, errors.New("session not found")
	}

	// Require the provider account associated with the Session.
	sessRef := entry.GetSessionRef()
	provRef := sessRef.GetProviderResourceRef()
	accountID := provRef.GetProviderAccountId()
	if accountID == "" {
		return nil, errors.New("session has no provider account id")
	}

	// Read the entity identity and challenge token for credential verification.
	entityID := req.GetEntityId()
	turnstileToken := req.GetTurnstileToken()

	// Derive entity keypair from the credential.
	var entityPriv crypto.PrivKey
	switch cred := req.GetCredential().(type) {
	case *s4wave_provider_spacewave.ReauthenticateSessionRequest_Password:
		password := cred.Password.GetPassword()
		if password == "" {
			return nil, errors.New("password is required")
		}
		if entityID == "" {
			return nil, errors.New("entity_id is required for password credential")
		}
		_, entityPriv, err = auth_method_password.BuildParametersWithUsernamePassword(entityID, []byte(password))
		if err != nil {
			return nil, errors.Wrap(err, "derive entity keypair")
		}
	case *s4wave_provider_spacewave.ReauthenticateSessionRequest_Pem:
		pemData := cred.Pem.GetPemData()
		if len(pemData) == 0 {
			return nil, errors.New("pem_data is required")
		}
		entityPriv, err = keypem.ParsePrivKeyPem(pemData)
		if err != nil {
			return nil, errors.Wrap(err, "parse PEM private key")
		}
		if entityPriv == nil {
			return nil, errors.New("pem_data must contain a PEM private key")
		}
	default:
		return nil, errors.New("credential is required")
	}

	// Derive the entity signing identity for cloud verification.
	entityPeerID, err := peer.IDFromPrivateKey(entityPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive entity peer id")
	}

	// Verify credentials with cloud by registering a probe session.
	entityCli := provider_spacewave.NewEntityClientDirect(
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		s.provider.GetSigningEnvPrefix(),
		entityPriv,
		entityPeerID,
	)
	_, err = entityCli.RegisterSessionDirectWithResponse(ctx, entityPeerID.String(), "reauth-probe", entityID, turnstileToken)
	if err != nil {
		return nil, errors.Wrap(err, "verify credentials with cloud")
	}

	// Store the entity key on the provider for the account tracker.
	bootstrapRef := s.provider.RetainEntityKeyBootstrap(accountID, entityPriv, entityPeerID)
	defer bootstrapRef.Release()

	// Access the provider account to perform key rotation.
	provAcc, provAccRef, err := provider.ExAccessProviderAccount(
		ctx, s.b,
		provRef.GetProviderId(),
		accountID,
		false, nil,
	)
	if err != nil {
		return nil, errors.Wrap(err, "access provider account")
	}
	defer provAccRef.Release()

	// Require a cloud provider account before rotating its Session key.
	swAcc, ok := provAcc.(*provider_spacewave.ProviderAccount)
	if !ok {
		return nil, errors.New("provider account is not a spacewave account")
	}

	// Generate a new Ed25519 session key.
	sessionPriv, _, err := crypto.GenerateEd25519Key(crand.Reader)
	if err != nil {
		return nil, errors.Wrap(err, "generate session key")
	}

	// Derive the peer identity for the replacement Session key.
	sessionPeerID, err := peer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		return nil, errors.Wrap(err, "derive session peer id")
	}

	// Register the new session peer with the cloud.
	if err := entityCli.RegisterSessionDirect(ctx, sessionPeerID.String(), "reauth"); err != nil {
		return nil, errors.Wrap(err, "register new session with cloud")
	}

	// Write the new encrypted key to ObjectStore.
	sessionID := provRef.GetId()
	volID := swAcc.GetVolume().GetID()
	objectStoreID := provider_spacewave.SessionObjectStoreID(accountID)
	objStoreHandle, _, diRef, err := volume.ExBuildObjectStoreAPI(ctx, s.b, false, objectStoreID, volID, nil)
	if err != nil {
		return nil, errors.Wrap(err, "mount session object store")
	}
	defer diRef.Release()
	objStore := objStoreHandle.GetObjectStore()

	// Derive storage key from volume peer key.
	volPeer, err := swAcc.GetVolume().GetPeer(ctx, true)
	if err != nil {
		return nil, errors.Wrap(err, "get volume peer")
	}
	volPrivKey, err := volPeer.GetPrivKey(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "get volume priv key")
	}
	storageKey, err := session_lock.DeriveStorageKey(volPrivKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive storage key")
	}

	// Encode the replacement Session key and scrub the temporary PEM.
	privPEM, err := keypem.MarshalPrivKeyPem(sessionPriv)
	if err != nil {
		return nil, errors.Wrap(err, "marshal session key")
	}
	defer scrub.Scrub(privPEM)

	// Encrypt and persist the replacement Session key for automatic unlock.
	encPriv, err := session_lock.EncryptAutoUnlock(storageKey, privPEM)
	if err != nil {
		return nil, errors.Wrap(err, "encrypt session key")
	}
	if err := session_lock.WriteAutoUnlock(ctx, objStore, sessionID, encPriv); err != nil {
		return nil, errors.Wrap(err, "write session key")
	}

	// Clear the registration state so the session tracker re-registers.
	regKey := []byte(sessionID + "/registered")
	if err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) {
			return objStore.NewTransaction(ctx, true)
		},
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, regKey, []byte(sessionPeerID.String()))
		},
	); err != nil {
		return nil, errors.Wrap(err, "persist session registration state")
	}

	// Replace SessionClient on the ProviderAccount with the new key.
	swAcc.ReplaceSessionClient(provider_spacewave.NewSessionClient(
		s.provider.GetHTTPClient(),
		s.provider.GetEndpoint(),
		s.provider.GetSigningEnvPrefix(),
		sessionPriv,
		sessionPeerID.String(),
	))

	// Clear UNAUTHENTICATED status and trigger a fresh fetch.
	swAcc.SetAccountStatus(provider.ProviderAccountStatus_ProviderAccountStatus_READY)
	swAcc.BumpLocalEpoch()

	return &s4wave_provider_spacewave.ReauthenticateSessionResponse{
		AccountStatus: provider.ProviderAccountStatus_ProviderAccountStatus_READY,
	}, nil
}

// _ is a type assertion
var _ s4wave_provider_spacewave.SRPCSpacewaveProviderResourceServiceServer = (*SpacewaveProviderResource)(nil)
