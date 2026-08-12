package provider_spacewave_handoff

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/aperturerobotics/fastjson"
	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"golang.org/x/crypto/hkdf"
)

// deviceEncryptInfo is the HKDF info string matching the TS implementation.
const (
	deviceEncryptInfo        = "spacewave-desktop-sso-v1"
	deviceEncryptedKeyMaxLen = 1024 * 1024
)

// SSOResult is the SSO result received from the Worker via WS relay.
type SSOResult struct {
	Linked          bool   `json:"linked"`
	Provider        string `json:"provider"`
	Email           string `json:"email"`
	Sub             string `json:"sub"`
	AccountID       string `json:"accountId,omitempty"`
	EntityID        string `json:"entityId,omitempty"`
	Username        string `json:"username,omitempty"`
	EncryptedBlob   string `json:"encryptedBlob,omitempty"`
	PinWrapped      bool   `json:"pinWrapped,omitempty"`
	DeviceEncrypted bool   `json:"deviceEncrypted,omitempty"`
	Error           string `json:"error,omitempty"`
}

// ssoProviderHosts lists the authorization hosts that StartSSOHandoff will
// accept in the server-returned openUrl. Any other host is rejected to prevent
// the cloud (or a MITM capable of swapping the response) from steering the
// desktop app to an arbitrary browser URL.
var ssoProviderHosts = map[string][]string{
	"google": {"accounts.google.com"},
	"github": {"github.com"},
}

// StartSSOHandoff initiates desktop SSO by opening the system browser
// to the SSO entry URL and waiting for the result via WebSocket relay.
//
// Returns the SSO result. For linked accounts, the entity key is decrypted.
// For new accounts, the caller handles account creation. The cloud-returned
// openUrl must be https and point at the provider's known authorize host.
func StartSSOHandoff(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	provider string,
) (*SSOResult, []byte, string, error) {
	var wsTicket string
	var nonce string
	cleanupSession := func() {}
	defer func() {
		cleanupSession()
	}()

	// 1. Generate ephemeral X25519 keypair.
	x25519Priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "generate X25519 keypair")
	}
	x25519PubRaw := x25519Priv.PublicKey().Bytes()

	// 2. POST /auth/sso/start with provider and device public key.
	startReq := &api.DesktopSSOStartRequest{
		Provider:        provider,
		DevicePublicKey: x25519PubRaw,
	}
	startBody, err := startReq.MarshalVT()
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "marshal desktop sso start request")
	}
	startURL := endpoint + "/api/auth/sso/start"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, startURL, bytes.NewReader(startBody))
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "build desktop sso start request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("Accept", "application/octet-stream")
	httpResp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "start desktop sso")
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(httpResp)
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "read desktop sso start response")
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, nil, "", errors.Errorf("desktop sso start failed: %d: %s", httpResp.StatusCode, string(respBody))
	}
	var startResp api.DesktopSSOStartResponse
	if err := startResp.UnmarshalVT(respBody); err != nil {
		return nil, nil, "", errors.Wrap(err, "parse desktop sso start response")
	}
	wsTicket = startResp.GetWsTicket()
	if wsTicket == "" {
		return nil, nil, "", errors.New("server did not return a desktop sso websocket ticket")
	}
	openURL := startResp.GetOpenUrl()
	if openURL == "" {
		return nil, nil, "", errors.New("server did not return a desktop sso browser url")
	}
	nonce, err = parseJWTSubject(wsTicket)
	if err != nil {
		return nil, nil, "", errors.Wrap(err, "parse desktop sso nonce")
	}
	cleanupSession = func() {
		cleanupAuthSession(httpCli, endpoint, nonce, wsTicket)
	}

	// 3. Connect WebSocket for result relay.
	wsURL := strings.Replace(endpoint, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/auth/session/ws?tk=" + wsTicket

	// 4. Open system browser to SSO entry URL.
	allowedHosts := ssoProviderHosts[provider]
	if len(allowedHosts) == 0 {
		return nil, nil, "", errors.Errorf("unsupported sso provider %q", provider)
	}
	if openErr := openBrowserValidated(openURL, allowedHosts); openErr != nil {
		return nil, nil, "", errors.Wrap(openErr, "open browser for SSO")
	}

	// 5. Wait for SSO result via WS, falling back to HTTP exchange on read error.
	// WS and HTTP both converge into the local SSOResult.
	var resultPtr *SSOResult
	for attempts := 0; ; attempts++ {
		conn, _, dialErr := websocket.Dial(ctx, wsURL, nil)
		if dialErr != nil {
			return nil, nil, "", errors.Wrap(dialErr, "connect websocket")
		}

		_, msg, readErr := conn.Read(ctx)
		_ = conn.Close(websocket.StatusNormalClosure, "")
		if readErr == nil {
			parsed, parseErr := parseSSOResult(msg)
			if parseErr != nil {
				return nil, nil, "", errors.Wrap(parseErr, "parse SSO result")
			}
			resultPtr = parsed
			break
		}
		if ctx.Err() != nil {
			return nil, nil, "", errors.Wrap(readErr, "read SSO result")
		}
		exchangeCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		exchanged, exchangeErr := exchangeAuthSessionSignInResult(
			exchangeCtx,
			httpCli,
			endpoint,
			nonce,
		)
		cancel()
		if exchangeErr == nil && exchanged != nil {
			resultPtr = ssoResultFromProto(exchanged)
			break
		}
		if attempts >= 2 {
			if exchangeErr != nil {
				return nil, nil, "", errors.Wrapf(
					readErr,
					"read SSO result after exchange fallback %v",
					exchangeErr,
				)
			}
			return nil, nil, "", errors.Wrap(readErr, "read SSO result")
		}
		select {
		case <-ctx.Done():
			return nil, nil, "", errors.Wrap(readErr, "read SSO result")
		case <-time.After(250 * time.Millisecond):
		}
	}

	result := *resultPtr

	if result.Error != "" {
		return &result, nil, nonce, errors.Errorf("SSO error: %s", result.Error)
	}

	// 6. If linked and device-encrypted, decrypt the entity key.
	if result.Linked && result.DeviceEncrypted && result.EncryptedBlob != "" {
		entityKeyPEM, decErr := decryptDesktopDeviceEncrypted(x25519Priv.Bytes(), result.EncryptedBlob)
		if decErr != nil {
			return &result, nil, nonce, errors.Wrap(decErr, "decrypt entity key")
		}
		cleanupSession = func() {}
		return &result, entityKeyPEM, nonce, nil
	}

	// If linked but not device-encrypted (no pubkey stored), return raw blob.
	if result.Linked && result.EncryptedBlob != "" {
		blob, decErr := base64.StdEncoding.DecodeString(result.EncryptedBlob)
		if decErr != nil {
			return &result, nil, nonce, errors.Wrap(decErr, "decode entity key blob")
		}
		cleanupSession = func() {}
		return &result, blob, nonce, nil
	}

	// Not linked (new user) - return result without entity key.
	cleanupSession = func() {}
	return &result, nil, nonce, nil
}

// DecryptDeviceEncrypted decrypts a protobuf browser device-key envelope.
func DecryptDeviceEncrypted(privateKeyRaw []byte, encryptedBlob string) ([]byte, error) {
	if base64.StdEncoding.DecodedLen(len(encryptedBlob)) > deviceEncryptedKeyMaxLen {
		return nil, errors.New("encrypted device key exceeds maximum size")
	}
	encoded, err := base64.StdEncoding.DecodeString(encryptedBlob)
	if err != nil {
		return nil, errors.Wrap(err, "decode encrypted device key")
	}
	var encrypted api.DeviceEncryptedKey
	if err := encrypted.UnmarshalVT(encoded); err != nil {
		return nil, errors.Wrap(err, "parse encrypted device key")
	}
	return decryptDeviceKey(
		privateKeyRaw,
		encrypted.GetEphemeralPublicKey(),
		encrypted.GetNonce(),
		encrypted.GetCiphertext(),
	)
}

func decryptDesktopDeviceEncrypted(privateKeyRaw []byte, encryptedBlob string) ([]byte, error) {
	if len(encryptedBlob) > base64.StdEncoding.EncodedLen(deviceEncryptedKeyMaxLen)+256 {
		return nil, errors.New("desktop encrypted device key exceeds maximum size")
	}
	var parser fastjson.Parser
	value, err := parser.Parse(encryptedBlob)
	if err != nil {
		return nil, errors.Wrap(err, "parse desktop encrypted device key")
	}
	ephemeralPublicKeyBase64 := value.GetStringBytes("ephemeralPublicKey")
	if len(ephemeralPublicKeyBase64) > base64.StdEncoding.EncodedLen(32) {
		return nil, errors.New("desktop ephemeral public key exceeds maximum size")
	}
	ephemeralPublicKey, err := base64.StdEncoding.DecodeString(string(ephemeralPublicKeyBase64))
	if err != nil {
		return nil, errors.Wrap(err, "decode desktop ephemeral public key")
	}
	nonceBase64 := value.GetStringBytes("iv")
	if len(nonceBase64) > base64.StdEncoding.EncodedLen(12) {
		return nil, errors.New("desktop nonce exceeds maximum size")
	}
	nonce, err := base64.StdEncoding.DecodeString(string(nonceBase64))
	if err != nil {
		return nil, errors.Wrap(err, "decode desktop nonce")
	}
	ciphertextBase64 := value.GetStringBytes("ciphertext")
	if len(ciphertextBase64) > base64.StdEncoding.EncodedLen(deviceEncryptedKeyMaxLen) {
		return nil, errors.New("desktop ciphertext exceeds maximum size")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(string(ciphertextBase64))
	if err != nil {
		return nil, errors.Wrap(err, "decode desktop ciphertext")
	}
	return decryptDeviceKey(privateKeyRaw, ephemeralPublicKey, nonce, ciphertext)
}

func decryptDeviceKey(privateKeyRaw, ephemeralPublicKey, nonce, ciphertext []byte) ([]byte, error) {
	if len(ephemeralPublicKey) != 32 {
		return nil, errors.New("encrypted device key has invalid ephemeral public key")
	}
	if len(nonce) != 12 {
		return nil, errors.New("encrypted device key has invalid nonce")
	}
	if len(ciphertext) < 16 || len(ciphertext) > deviceEncryptedKeyMaxLen {
		return nil, errors.New("encrypted device key has invalid ciphertext")
	}
	privateKey, err := ecdh.X25519().NewPrivateKey(privateKeyRaw)
	if err != nil {
		return nil, errors.Wrap(err, "parse device private key")
	}
	ephemeralKey, err := ecdh.X25519().NewPublicKey(ephemeralPublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "parse ephemeral public key")
	}
	sharedSecret, err := privateKey.ECDH(ephemeralKey)
	if err != nil {
		return nil, errors.Wrap(err, "derive shared secret")
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(hkdf.New(sha256.New, sharedSecret, make([]byte, 32), []byte(deviceEncryptInfo)), key); err != nil {
		return nil, errors.Wrap(err, "derive encryption key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, errors.Wrap(err, "create AES cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "create AES-GCM")
	}
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "decrypt AES-GCM ciphertext")
	}
	return plaintext, nil
}

// ssoResultFromProto converts a typed SSOCodeExchangeResponse proto returned
// from the HTTP exchange fallback into the local SSOResult struct used by
// StartSSOHandoff. The WS path still receives JSON and uses parseSSOResult.
func ssoResultFromProto(resp *api.SSOCodeExchangeResponse) *SSOResult {
	return &SSOResult{
		Linked:          resp.GetLinked(),
		Provider:        resp.GetProvider(),
		Email:           resp.GetEmail(),
		Sub:             resp.GetSub(),
		AccountID:       resp.GetAccountId(),
		EntityID:        resp.GetEntityId(),
		Username:        resp.GetUsername(),
		EncryptedBlob:   resp.GetEncryptedBlob(),
		PinWrapped:      resp.GetPinWrapped(),
		DeviceEncrypted: resp.GetDeviceEncrypted(),
		Error:           resp.GetError(),
	}
}

func parseSSOResult(dat []byte) (*SSOResult, error) {
	var frame api.WsAuthSessionServerFrame
	if err := frame.UnmarshalVT(dat); err != nil {
		return nil, err
	}
	body, ok := frame.GetBody().(*api.WsAuthSessionServerFrame_SsoCallback)
	if !ok || body.SsoCallback == nil {
		return nil, errors.New("auth-session frame missing SSO callback")
	}
	v := body.SsoCallback
	return &SSOResult{
		Linked:          v.GetLinked(),
		Provider:        v.GetProvider(),
		Email:           v.GetEmail(),
		Sub:             v.GetSub(),
		AccountID:       v.GetAccountId(),
		EntityID:        v.GetEntityId(),
		Username:        v.GetUsername(),
		EncryptedBlob:   v.GetEncryptedBlob(),
		PinWrapped:      v.GetPinWrapped(),
		DeviceEncrypted: v.GetDeviceEncrypted(),
		Error:           v.GetError(),
	}, nil
}

func parseJWTSubject(jwt string) (string, error) {
	parts := strings.Split(jwt, ".")
	if len(parts) != 3 {
		return "", errors.New("invalid jwt format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", errors.Wrap(err, "decode jwt payload")
	}
	var p fastjson.Parser
	v, err := p.ParseBytes(payload)
	if err != nil {
		return "", errors.Wrap(err, "parse jwt payload")
	}
	sub := string(v.GetStringBytes("sub"))
	if sub == "" {
		return "", errors.New("jwt subject is required")
	}
	return sub, nil
}
