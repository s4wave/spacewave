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
	"log"
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
const deviceEncryptInfo = "spacewave-desktop-sso-v1"

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

// encryptedForDevice matches the TS EncryptedForDevice structure.
type encryptedForDevice struct {
	EphemeralPublicKey string `json:"ephemeralPublicKey"`
	IV                 string `json:"iv"`
	Ciphertext         string `json:"ciphertext"`
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
		delCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		if err := deleteAuthSession(delCtx, httpCli, endpoint, nonce, wsTicket); err != nil {
			log.Println("warning: failed to delete SSO auth session:", err)
		}
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
		entityKeyPEM, decErr := decryptDeviceEncrypted(x25519Priv, result.EncryptedBlob)
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

// decryptDeviceEncrypted decrypts an entity key that was encrypted
// by the Worker using X25519 ECDH + HKDF-SHA256 + AES-256-GCM.
func decryptDeviceEncrypted(privKey *ecdh.PrivateKey, encryptedJSON string) ([]byte, error) {
	enc, err := parseEncryptedForDevice(encryptedJSON)
	if err != nil {
		return nil, errors.Wrap(err, "parse encrypted structure")
	}

	ephPubRaw, err := base64.StdEncoding.DecodeString(enc.EphemeralPublicKey)
	if err != nil {
		return nil, errors.Wrap(err, "decode ephemeral public key")
	}
	iv, err := base64.StdEncoding.DecodeString(enc.IV)
	if err != nil {
		return nil, errors.Wrap(err, "decode IV")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(enc.Ciphertext)
	if err != nil {
		return nil, errors.Wrap(err, "decode ciphertext")
	}

	// Import the ephemeral public key.
	ephPubKey, err := ecdh.X25519().NewPublicKey(ephPubRaw)
	if err != nil {
		return nil, errors.Wrap(err, "import ephemeral public key")
	}

	// ECDH to derive shared secret.
	sharedSecret, err := privKey.ECDH(ephPubKey)
	if err != nil {
		return nil, errors.Wrap(err, "ECDH key exchange")
	}

	// HKDF-SHA256 to derive AES-256 key.
	hkdfReader := hkdf.New(sha256.New, sharedSecret, make([]byte, 32), []byte(deviceEncryptInfo))
	aesKey := make([]byte, 32)
	if _, err := io.ReadFull(hkdfReader, aesKey); err != nil {
		return nil, errors.Wrap(err, "HKDF derive key")
	}

	// AES-256-GCM decrypt.
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, errors.Wrap(err, "create AES cipher")
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.Wrap(err, "create GCM")
	}
	plaintext, err := gcm.Open(nil, iv, ciphertext, nil)
	if err != nil {
		return nil, errors.Wrap(err, "AES-GCM decrypt")
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

func parseEncryptedForDevice(dat string) (*encryptedForDevice, error) {
	var p fastjson.Parser
	v, err := p.Parse(dat)
	if err != nil {
		return nil, err
	}
	return &encryptedForDevice{
		EphemeralPublicKey: string(v.GetStringBytes("ephemeralPublicKey")),
		IV:                 string(v.GetStringBytes("iv")),
		Ciphertext:         string(v.GetStringBytes("ciphertext")),
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
