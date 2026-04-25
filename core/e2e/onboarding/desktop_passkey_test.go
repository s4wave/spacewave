//go:build e2e

package onboarding_test

import (
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	provider_spacewave_api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	provider_spacewave_handoff "github.com/s4wave/spacewave/core/provider/spacewave/handoff"
	resource_account "github.com/s4wave/spacewave/core/resource/account"
	resource_provider "github.com/s4wave/spacewave/core/resource/provider"
	resource_session "github.com/s4wave/spacewave/core/resource/session"
	core_session "github.com/s4wave/spacewave/core/session"
	bifcrypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	bifpeer "github.com/s4wave/spacewave/net/peer"
	s4wave_account "github.com/s4wave/spacewave/sdk/account"
	s4wave_provider_spacewave "github.com/s4wave/spacewave/sdk/provider/spacewave"
	"github.com/sirupsen/logrus"
)

func newAccountHostRequest(
	ctx context.Context,
	method string,
	host string,
	path string,
	body []byte,
) (*http.Response, []byte, error) {
	return newAccountHostRequestCT(ctx, method, host, path, body, "application/json")
}

func newAccountHostRequestBinary(
	ctx context.Context,
	method string,
	host string,
	path string,
	body []byte,
) (*http.Response, []byte, error) {
	return newAccountHostRequestCT(
		ctx,
		method,
		host,
		path,
		body,
		"application/octet-stream",
	)
}

func newAccountHostRequestCT(
	ctx context.Context,
	method string,
	host string,
	path string,
	body []byte,
	contentType string,
) (*http.Response, []byte, error) {
	reqURL := env.cloudURL + path
	req, err := http.NewRequestWithContext(
		ctx,
		method,
		reqURL,
		bytes.NewReader(body),
	)
	if err != nil {
		return nil, nil, err
	}
	req.Host = host
	if len(body) != 0 {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	respBody, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return nil, nil, err
	}
	return resp, respBody, nil
}

func loadAccountHostCeremony(
	ctx context.Context,
	t *testing.T,
	rawURL string,
) (string, string) {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("parse browser url: %v", err)
	}
	resp, body, err := newAccountHostRequest(
		ctx,
		http.MethodGet,
		u.Host,
		u.RequestURI(),
		nil,
	)
	if err != nil {
		t.Fatalf("load ceremony page: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("ceremony page returned %d: %s", resp.StatusCode, string(body))
	}
	if !strings.Contains(string(body), "<html") {
		t.Fatalf("ceremony page did not return HTML: %s", string(body))
	}
	nonce := u.Query().Get("nonce")
	if nonce == "" {
		t.Fatal("ceremony url missing nonce")
	}
	return u.Host, nonce
}

func newConfiguredVirtualAuthenticator(t *testing.T) *virtualAuthenticator {
	t.Helper()
	va, err := newVirtualAuthenticatorWithOrigin(
		env.passkeyOrigin,
		env.passkeyRpID,
	)
	if err != nil {
		t.Fatalf("create virtual authenticator: %v", err)
	}
	return va
}

func lookupSpacewaveProvider(
	ctx context.Context,
	t *testing.T,
) (*provider_spacewave.Provider, func()) {
	t.Helper()
	prov, provRef, err := provider.ExLookupProvider(
		ctx,
		env.tb.Bus,
		"spacewave",
		false,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	swProv, ok := prov.(*provider_spacewave.Provider)
	if !ok {
		provRef.Release()
		t.Fatal("expected spacewave provider")
	}
	return swProv, provRef.Release
}

func buildSpacewaveProviderResource(
	ctx context.Context,
	t *testing.T,
) *resource_provider.SpacewaveProviderResource {
	t.Helper()
	swProv, rel := lookupSpacewaveProvider(ctx, t)
	t.Cleanup(rel)
	le := logrus.NewEntry(logrus.StandardLogger())
	parent := resource_provider.NewProviderResource(le, env.tb.Bus, swProv)
	return resource_provider.NewSpacewaveProviderResource(parent, le, env.tb.Bus, swProv)
}

func accessSpacewaveAccount(
	ctx context.Context,
	t *testing.T,
	accountID string,
) (*provider_spacewave.ProviderAccount, func()) {
	t.Helper()
	swProv, relProv := lookupSpacewaveProvider(ctx, t)
	accIface, relAcc, err := swProv.AccessProviderAccount(ctx, accountID, nil)
	if err != nil {
		relProv()
		t.Fatal(err)
	}
	acc, ok := accIface.(*provider_spacewave.ProviderAccount)
	if !ok {
		relAcc()
		relProv()
		t.Fatal("expected spacewave provider account")
	}
	return acc, func() {
		relAcc()
		relProv()
	}
}

func getAccountUsername(
	ctx context.Context,
	t *testing.T,
	acc *provider_spacewave.ProviderAccount,
) string {
	t.Helper()
	state, err := acc.GetAccountState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	username := state.GetEntityId()
	if username == "" {
		t.Fatal("account state missing entity id")
	}
	return username
}

func marshalEntityPEM(
	t *testing.T,
	priv bifcrypto.PrivKey,
) ([]byte, string) {
	t.Helper()
	pemDat, err := keypem.MarshalPrivKeyPem(priv)
	if err != nil {
		t.Fatalf("marshal entity pem: %v", err)
	}
	return pemDat, base64.StdEncoding.EncodeToString(pemDat)
}

func generateEntityAndSessionKeys(
	t *testing.T,
) ([]byte, string, string) {
	t.Helper()
	entityPriv, _, err := bifcrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate entity key: %v", err)
	}
	entityPeerID, err := bifpeer.IDFromPrivateKey(entityPriv)
	if err != nil {
		t.Fatalf("derive entity peer id: %v", err)
	}
	entityPEM, err := keypem.MarshalPrivKeyPem(entityPriv)
	if err != nil {
		t.Fatalf("marshal entity pem: %v", err)
	}

	sessionPriv, _, err := bifcrypto.GenerateEd25519Key(nil)
	if err != nil {
		t.Fatalf("generate session key: %v", err)
	}
	sessionPeerID, err := bifpeer.IDFromPrivateKey(sessionPriv)
	if err != nil {
		t.Fatalf("derive session peer id: %v", err)
	}
	return entityPEM, entityPeerID.String(), sessionPeerID.String()
}

func registerPasskeyForAccount(
	ctx context.Context,
	t *testing.T,
	cli *provider_spacewave.SessionClient,
	entityPriv bifcrypto.PrivKey,
	entityPeerID bifpeer.ID,
	va *virtualAuthenticator,
) {
	t.Helper()
	optionsJSON, err := cli.PasskeyRegisterOptions(ctx)
	if err != nil {
		t.Fatalf("passkey register options: %v", err)
	}
	challenge := extractJSONField(optionsJSON, "challenge")
	if challenge == "" {
		t.Fatal("registration options missing challenge")
	}
	credJSON := va.createRegistrationResponse(challenge)
	_, wrappedEntityKey := marshalEntityPEM(t, entityPriv)
	if _, err := cli.PasskeyRegisterVerify(
		ctx,
		credJSON,
		false,
		wrappedEntityKey,
		entityPeerID.String(),
		"",
		"",
	); err != nil {
		t.Fatalf("passkey register verify: %v", err)
	}
}

func simulateDesktopPasskeyLoginBrowser(
	ctx context.Context,
	t *testing.T,
	rawURL string,
	username string,
	va *virtualAuthenticator,
) error {
	t.Helper()
	host, nonce := loadAccountHostCeremony(ctx, t, rawURL)
	optionsJSON, err := provider_spacewave.PasskeyAuthOptions(
		ctx,
		httpClient,
		env.cloudURL,
		username,
	)
	if err != nil {
		return err
	}
	challenge := extractJSONField(optionsJSON, "challenge")
	if challenge == "" {
		t.Fatal("passkey auth options missing challenge")
	}
	credentialJSON := va.createAuthenticationResponse(challenge)
	verifyResp, err := provider_spacewave.PasskeyAuthVerify(
		ctx,
		httpClient,
		env.cloudURL,
		credentialJSON,
	)
	if err != nil {
		return err
	}
	req := &provider_spacewave_api.DesktopPasskeyRelayResult{
		Nonce: nonce,
		Result: &provider_spacewave_api.DesktopPasskeyRelayResult_Linked{
			Linked: &provider_spacewave_api.DesktopPasskeyLinkedResult{
				EncryptedBlob: verifyResp.GetEncryptedBlob(),
				PrfCapable:    verifyResp.GetPrfCapable(),
				PrfSalt:       verifyResp.GetPrfSalt(),
				AuthParams:    verifyResp.GetAuthParams(),
				PinWrapped:    verifyResp.GetPinWrapped(),
			},
		},
	}
	return relayDesktopPasskey(ctx, host, req)
}

func simulateDesktopPasskeySignupBrowser(
	ctx context.Context,
	t *testing.T,
	rawURL string,
	username string,
	va *virtualAuthenticator,
) error {
	t.Helper()
	host, nonce := loadAccountHostCeremony(ctx, t, rawURL)
	optionsJSON, err := provider_spacewave.PasskeyRegisterChallenge(
		ctx,
		httpClient,
		env.cloudURL,
		username,
	)
	if err != nil {
		return err
	}
	challenge := extractJSONField(optionsJSON, "challenge")
	if challenge == "" {
		t.Fatal("passkey register challenge missing challenge")
	}
	credentialJSON := va.createRegistrationResponse(challenge)
	req := &provider_spacewave_api.DesktopPasskeyRelayResult{
		Nonce: nonce,
		Result: &provider_spacewave_api.DesktopPasskeyRelayResult_NewAccount{
			NewAccount: &provider_spacewave_api.DesktopPasskeyNewAccountResult{
				Username:       username,
				CredentialJson: credentialJSON,
				PrfCapable:     false,
			},
		},
	}
	return relayDesktopPasskey(ctx, host, req)
}

func relayDesktopPasskey(
	ctx context.Context,
	host string,
	req *provider_spacewave_api.DesktopPasskeyRelayResult,
) error {
	body, err := req.MarshalVT()
	if err != nil {
		return err
	}
	resp, respBody, err := newAccountHostRequestBinary(
		ctx,
		http.MethodPost,
		host,
		"/api/auth/passkey/desktop/relay",
		body,
	)
	if err != nil {
		return err
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf(
			"desktop passkey relay returned %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}
	return nil
}

func fetchDesktopPasskeyRegisterChallenge(
	ctx context.Context,
	t *testing.T,
	host string,
	nonce string,
) *provider_spacewave_api.DesktopPasskeyRegisterChallengeResponse {
	t.Helper()
	req := &provider_spacewave_api.DesktopPasskeyRegisterChallengeRequest{
		Nonce: nonce,
	}
	body, err := req.MarshalVT()
	if err != nil {
		t.Fatalf("marshal desktop passkey register challenge: %v", err)
	}
	resp, respBody, err := newAccountHostRequestBinary(
		ctx,
		http.MethodPost,
		host,
		"/api/auth/passkey/desktop/register/challenge",
		body,
	)
	if err != nil {
		t.Fatalf("desktop passkey register challenge: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"desktop passkey register challenge returned %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}
	out := &provider_spacewave_api.DesktopPasskeyRegisterChallengeResponse{}
	if err := out.UnmarshalVT(respBody); err != nil {
		t.Fatalf("unmarshal desktop passkey register challenge: %v", err)
	}
	return out
}

func relayDesktopPasskeyRegister(
	ctx context.Context,
	t *testing.T,
	host string,
	req *provider_spacewave_api.DesktopPasskeyRegisterRelayResult,
) {
	t.Helper()
	body, err := req.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal desktop passkey register relay: %v", err)
	}
	resp, respBody, err := newAccountHostRequest(
		ctx,
		http.MethodPost,
		host,
		"/api/auth/passkey/desktop/register/relay",
		body,
	)
	if err != nil {
		t.Fatalf("desktop passkey register relay: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"desktop passkey register relay returned %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}
}

func simulateDesktopPasskeyRegisterBrowser(
	ctx context.Context,
	t *testing.T,
	rawURL string,
	va *virtualAuthenticator,
) error {
	host, nonce := loadAccountHostCeremony(ctx, t, rawURL)
	chalResp := fetchDesktopPasskeyRegisterChallenge(ctx, t, host, nonce)
	challenge := extractJSONField(chalResp.GetOptionsJson(), "challenge")
	if challenge == "" {
		t.Fatal("desktop register challenge missing challenge")
	}
	credentialJSON := va.createRegistrationResponse(challenge)
	relayDesktopPasskeyRegister(
		ctx,
		t,
		host,
		&provider_spacewave_api.DesktopPasskeyRegisterRelayResult{
			Nonce: nonce,
			Register: &provider_spacewave_api.DesktopPasskeyRegisterResult{
				Username:       chalResp.GetUsername(),
				CredentialJson: credentialJSON,
				PrfCapable:     false,
			},
		},
	)
	return nil
}

func verifyDesktopPasskeyReauth(
	ctx context.Context,
	t *testing.T,
	host string,
	nonce string,
	credentialJSON string,
) *provider_spacewave_api.DesktopPasskeyReauthResult {
	t.Helper()
	req := &provider_spacewave_api.DesktopPasskeyReauthVerifyRequest{
		Nonce:          nonce,
		CredentialJson: credentialJSON,
	}
	body, err := req.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal desktop passkey reauth verify: %v", err)
	}
	resp, respBody, err := newAccountHostRequest(
		ctx,
		http.MethodPost,
		host,
		"/api/auth/passkey/desktop/reauth/verify",
		body,
	)
	if err != nil {
		t.Fatalf("desktop passkey reauth verify: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"desktop passkey reauth verify returned %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}
	out := &provider_spacewave_api.DesktopPasskeyReauthResult{}
	if err := out.UnmarshalJSON(respBody); err != nil {
		t.Fatalf("unmarshal desktop passkey reauth verify: %v", err)
	}
	return out
}

func relayDesktopPasskeyReauth(
	ctx context.Context,
	t *testing.T,
	host string,
	req *provider_spacewave_api.DesktopPasskeyReauthRelayResult,
) {
	t.Helper()
	body, err := req.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal desktop passkey reauth relay: %v", err)
	}
	resp, respBody, err := newAccountHostRequest(
		ctx,
		http.MethodPost,
		host,
		"/api/auth/passkey/desktop/reauth/relay",
		body,
	)
	if err != nil {
		t.Fatalf("desktop passkey reauth relay: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf(
			"desktop passkey reauth relay returned %d: %s",
			resp.StatusCode,
			string(respBody),
		)
	}
}

func simulateDesktopPasskeyReauthBrowser(
	ctx context.Context,
	t *testing.T,
	rawURL string,
	username string,
	va *virtualAuthenticator,
) error {
	host, nonce := loadAccountHostCeremony(ctx, t, rawURL)
	optionsJSON, err := provider_spacewave.PasskeyAuthOptions(
		ctx,
		httpClient,
		env.cloudURL,
		username,
	)
	if err != nil {
		return err
	}
	challenge := extractJSONField(optionsJSON, "challenge")
	if challenge == "" {
		t.Fatal("desktop passkey reauth options missing challenge")
	}
	credentialJSON := va.createAuthenticationResponse(challenge)
	verifyResp := verifyDesktopPasskeyReauth(ctx, t, host, nonce, credentialJSON)
	relayDesktopPasskeyReauth(
		ctx,
		t,
		host,
		&provider_spacewave_api.DesktopPasskeyReauthRelayResult{
			Nonce:  nonce,
			Reauth: verifyResp,
		},
	)
	return nil
}

func TestDesktopPasskeyLoginLinkedEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	cloudEntry, entityPriv, entityPeerID := createCloudSessionWithKey(ctx, t)
	accountID := cloudEntry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
	acc, relAcc := accessSpacewaveAccount(ctx, t, accountID)
	defer relAcc()
	username := getAccountUsername(ctx, t, acc)
	va := newConfiguredVirtualAuthenticator(t)
	registerPasskeyForAccount(ctx, t, acc.GetSessionClient(), entityPriv, entityPeerID, va)

	restore := provider_spacewave_handoff.SetBrowserOpenerForTesting(
		func(rawURL string) error {
			return simulateDesktopPasskeyLoginBrowser(ctx, t, rawURL, username, va)
		},
	)
	defer restore()

	swResource := buildSpacewaveProviderResource(ctx, t)
	startResp, err := swResource.StartDesktopPasskey(
		ctx,
		&s4wave_provider_spacewave.StartDesktopPasskeyRequest{},
	)
	if err != nil {
		t.Fatalf("start desktop passkey: %v", err)
	}
	linked := startResp.GetLinked()
	if linked == nil {
		t.Fatal("expected linked desktop passkey result")
	}
	if linked.GetEncryptedBlob() == "" {
		t.Fatal("linked desktop passkey result missing encrypted blob")
	}
	pemDat, err := base64.StdEncoding.DecodeString(linked.GetEncryptedBlob())
	if err != nil {
		t.Fatalf("decode linked desktop passkey pem: %v", err)
	}
	loginResp, err := swResource.LoginWithEntityKey(
		ctx,
		&s4wave_provider_spacewave.LoginWithEntityKeyRequest{
			PemPrivateKey: pemDat,
		},
	)
	if err != nil {
		t.Fatalf("login with entity key: %v", err)
	}
	entry := loginResp.GetSessionListEntry()
	if entry == nil {
		t.Fatal("login with entity key returned no session entry")
	}
	if entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() != accountID {
		t.Fatalf(
			"expected logged-in session for account %s, got %s",
			accountID,
			entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId(),
		)
	}
}

func TestDesktopPasskeyLoginNewAccountEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	username := "desktop-passkey-" + ulid.NewULID()
	va := newConfiguredVirtualAuthenticator(t)
	restore := provider_spacewave_handoff.SetBrowserOpenerForTesting(
		func(rawURL string) error {
			return simulateDesktopPasskeySignupBrowser(ctx, t, rawURL, username, va)
		},
	)
	defer restore()

	swResource := buildSpacewaveProviderResource(ctx, t)
	startResp, err := swResource.StartDesktopPasskey(
		ctx,
		&s4wave_provider_spacewave.StartDesktopPasskeyRequest{},
	)
	if err != nil {
		t.Fatalf("start desktop passkey signup: %v", err)
	}
	newAccount := startResp.GetNewAccount()
	if newAccount == nil {
		t.Fatal("expected new-account desktop passkey result")
	}
	entityPEM, entityPeerID, sessionPeerID := generateEntityAndSessionKeys(t)
	confirmResp, err := swResource.ConfirmDesktopPasskey(
		ctx,
		&s4wave_provider_spacewave.ConfirmDesktopPasskeyRequest{
			Nonce:            newAccount.GetNonce(),
			Username:         newAccount.GetUsername(),
			CredentialJson:   newAccount.GetCredentialJson(),
			WrappedEntityKey: base64.StdEncoding.EncodeToString(entityPEM),
			EntityPeerId:     entityPeerID,
			SessionPeerId:    sessionPeerID,
			PinWrapped:       false,
			PrfCapable:       false,
		},
	)
	if err != nil {
		t.Fatalf("confirm desktop passkey signup: %v", err)
	}
	if confirmResp.GetAccountId() == "" {
		t.Fatal("confirm desktop passkey signup returned no account id")
	}
	if confirmResp.GetSessionPeerId() != sessionPeerID {
		t.Fatalf(
			"expected session peer id %s, got %s",
			sessionPeerID,
			confirmResp.GetSessionPeerId(),
		)
	}

	loginResp, err := swResource.LoginWithEntityKey(
		ctx,
		&s4wave_provider_spacewave.LoginWithEntityKeyRequest{
			PemPrivateKey: entityPEM,
		},
	)
	if err != nil {
		t.Fatalf("login with new desktop passkey entity key: %v", err)
	}
	entry := loginResp.GetSessionListEntry()
	if entry == nil {
		t.Fatal("new desktop passkey login returned no session entry")
	}
	if entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() != confirmResp.GetAccountId() {
		t.Fatalf(
			"expected login for confirmed account %s, got %s",
			confirmResp.GetAccountId(),
			entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId(),
		)
	}
}

func TestDesktopPasskeyRegisterEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	cloudEntry, entityPriv, entityPeerID := createCloudSessionWithKey(ctx, t)
	accountID := cloudEntry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
	acc, relAcc := accessSpacewaveAccount(ctx, t, accountID)
	defer relAcc()
	accResource := resource_account.NewAccountResource(acc)
	defer accResource.Release()
	entityPEM, wrappedEntityKey := marshalEntityPEM(t, entityPriv)
	va := newConfiguredVirtualAuthenticator(t)

	restore := provider_spacewave_handoff.SetBrowserOpenerForTesting(
		func(rawURL string) error {
			return simulateDesktopPasskeyRegisterBrowser(ctx, t, rawURL, va)
		},
	)
	defer restore()

	handoffResp, err := accResource.StartDesktopPasskeyRegisterHandoff(
		ctx,
		&s4wave_account.StartDesktopPasskeyRegisterHandoffRequest{},
	)
	if err != nil {
		t.Fatalf("start desktop passkey register handoff: %v", err)
	}
	if handoffResp.GetCredentialJson() == "" {
		t.Fatal("desktop passkey register handoff returned empty credential json")
	}
	if _, err := accResource.PasskeyRegisterVerify(
		ctx,
		&s4wave_account.PasskeyRegisterVerifyRequest{
			CredentialJson:   handoffResp.GetCredentialJson(),
			PrfCapable:       handoffResp.GetPrfCapable(),
			EncryptedPrivkey: wrappedEntityKey,
			PeerId:           entityPeerID.String(),
			AuthParams:       "",
			PrfSalt:          handoffResp.GetPrfSalt(),
		},
	); err != nil {
		t.Fatalf("desktop passkey register verify: %v", err)
	}

	username := getAccountUsername(ctx, t, acc)
	optionsJSON, err := provider_spacewave.PasskeyAuthOptions(
		ctx,
		httpClient,
		env.cloudURL,
		username,
	)
	if err != nil {
		t.Fatalf("passkey auth options: %v", err)
	}
	challenge := extractJSONField(optionsJSON, "challenge")
	if challenge == "" {
		t.Fatal("passkey auth options missing challenge")
	}
	authResp, err := provider_spacewave.PasskeyAuthVerify(
		ctx,
		httpClient,
		env.cloudURL,
		va.createAuthenticationResponse(challenge),
	)
	if err != nil {
		t.Fatalf("passkey auth verify after desktop register: %v", err)
	}
	if authResp.GetAccountId() != accountID {
		t.Fatalf("expected auth verify for account %s, got %s", accountID, authResp.GetAccountId())
	}
	if authResp.GetEncryptedBlob() == "" {
		t.Fatal("passkey auth verify after desktop register returned no encrypted blob")
	}
	if len(entityPEM) == 0 {
		t.Fatal("entity pem unexpectedly empty")
	}
}

func TestDesktopPasskeyReauthEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()

	cloudEntry, entityPriv, entityPeerID := createCloudSessionWithKey(ctx, t)
	accountID := cloudEntry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId()
	acc, relAcc := accessSpacewaveAccount(ctx, t, accountID)
	defer relAcc()
	username := getAccountUsername(ctx, t, acc)
	va := newConfiguredVirtualAuthenticator(t)
	registerPasskeyForAccount(ctx, t, acc.GetSessionClient(), entityPriv, entityPeerID, va)

	cloudResource, cloudSess, relCloudResource := mountSessionResource(ctx, t, cloudEntry)
	defer relCloudResource()
	swResource := resource_session.NewSpacewaveSessionResource(
		cloudResource,
		logrus.NewEntry(logrus.StandardLogger()),
		env.tb.Bus,
		cloudSess,
		acc,
	)
	accResource := resource_account.NewAccountResource(acc)
	defer accResource.Release()

	restore := provider_spacewave_handoff.SetBrowserOpenerForTesting(
		func(rawURL string) error {
			return simulateDesktopPasskeyReauthBrowser(ctx, t, rawURL, username, va)
		},
	)
	defer restore()

	reauthResp, err := swResource.StartDesktopPasskeyReauth(
		ctx,
		&s4wave_provider_spacewave.StartDesktopPasskeyReauthRequest{
			PeerId: entityPeerID.String(),
		},
	)
	if err != nil {
		t.Fatalf("start desktop passkey reauth: %v", err)
	}
	if reauthResp.GetEncryptedBlob() == "" {
		t.Fatal("desktop passkey reauth returned no encrypted blob")
	}
	if reauthResp.GetPrfCapable() {
		t.Fatal("expected non-PRF desktop passkey reauth in e2e harness")
	}
	if reauthResp.GetPinWrapped() {
		t.Fatal("expected non-pin-wrapped desktop passkey reauth in e2e harness")
	}
	pemDat, err := base64.StdEncoding.DecodeString(reauthResp.GetEncryptedBlob())
	if err != nil {
		t.Fatalf("decode desktop passkey reauth pem: %v", err)
	}
	if _, err := accResource.UnlockEntityKeypair(
		ctx,
		&s4wave_account.UnlockEntityKeypairRequest{
			PeerId: entityPeerID.String(),
			Credential: &core_session.EntityCredential{
				Credential: &core_session.EntityCredential_PemPrivateKey{
					PemPrivateKey: pemDat,
				},
			},
		},
	); err != nil {
		t.Fatalf("unlock entity keypair from desktop passkey reauth: %v", err)
	}
	backupResp, err := accResource.GenerateBackupKey(
		ctx,
		&s4wave_account.GenerateBackupKeyRequest{},
	)
	if err != nil {
		t.Fatalf("generate backup key after desktop passkey unlock: %v", err)
	}
	if len(backupResp.GetPemData()) == 0 {
		t.Fatal("generate backup key returned empty pem")
	}
}
