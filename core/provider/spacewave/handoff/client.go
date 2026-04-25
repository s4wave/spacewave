package provider_spacewave_handoff

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/aperturerobotics/util/scrub"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	session_handoff "github.com/s4wave/spacewave/core/session/handoff"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
)

// StartHandoff initiates browser-delegated auth by opening a browser
// and waiting for the encrypted session key via WebSocket relay.
//
// Returns the decrypted session private key, account ID, and entity ID.
func StartHandoff(
	ctx context.Context,
	httpCli *http.Client,
	apiEndpoint string,
	publicBaseURL string,
	clientType string,
	authIntent string,
	username string,
) (crypto.PrivKey, string, string, error) {
	var nonce string
	var wsTicket string
	cleanupSession := func() {}
	defer func() {
		cleanupSession()
	}()

	// 1. Generate ephemeral Ed25519 keypair.
	ephPriv, ephPub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "generate ephemeral keypair")
	}

	ephPubRaw, err := ephPub.Raw()
	if err != nil {
		return nil, "", "", errors.Wrap(err, "get ephemeral public key bytes")
	}

	// 2. Build HandoffRequest proto.
	nonce = ulid.NewULID()
	deviceName := getDeviceName()
	if clientType == "" {
		clientType = "desktop"
	}
	handoffReq := &session_handoff.HandoffRequest{
		DevicePublicKey: ephPubRaw,
		DeviceName:      deviceName,
		SessionNonce:    nonce,
		ClientType:      clientType,
	}

	// 3. POST /auth/session/create -> {nonce, wsTicket}.
	createReq := &api.AuthSessionCreateRequest{Nonce: nonce}
	createBody, err := createReq.MarshalVT()
	if err != nil {
		return nil, "", "", errors.Wrap(err, "marshal create request")
	}
	createURL := buildCreateSessionURL(apiEndpoint)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, createURL, bytes.NewReader(createBody))
	if err != nil {
		return nil, "", "", errors.Wrap(err, "build create request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpResp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "create auth session")
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(httpResp)
	respBody, err := io.ReadAll(httpResp.Body)
	if httpResp.StatusCode != http.StatusOK {
		return nil, "", "", errors.Errorf("create auth session failed: %d: %s", httpResp.StatusCode, string(respBody))
	}
	var createResp api.AuthSessionCreateResponse
	if err := createResp.UnmarshalVT(respBody); err != nil {
		return nil, "", "", errors.Wrap(err, "parse create response")
	}
	wsTicket = createResp.GetWsTicket()
	if wsTicket == "" {
		return nil, "", "", errors.New("server did not return an auth session websocket ticket")
	}
	cleanupSession = func() {
		delCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		if err := deleteAuthSession(delCtx, httpCli, apiEndpoint, nonce, wsTicket); err != nil {
			log.Println("warning: failed to delete handoff auth session:", err)
		}
	}

	// 4. Open browser with handoff URL.
	reqData, err := handoffReq.MarshalVT()
	if err != nil {
		return nil, "", "", errors.Wrap(err, "marshal handoff request")
	}
	payload := base64.RawURLEncoding.EncodeToString(reqData)
	authURL := buildHandoffBrowserURL(publicBaseURL, payload, authIntent, username)
	if openErr := openBrowserValidated(authURL, hostsFromURLs(publicBaseURL)); openErr != nil {
		return nil, "", "", errors.Wrap(openErr, "open browser")
	}

	// 5. Connect WebSocket.
	wsURL := buildHandoffWSURL(apiEndpoint, wsTicket)
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "connect websocket")
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// 6. Wait for encrypted payload.
	_, msg, err := conn.Read(ctx)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "read handoff completion")
	}

	// 7. Parse HandoffCompletion.
	var frame api.WsAuthSessionServerFrame
	if err := frame.UnmarshalVT(msg); err != nil {
		return nil, "", "", errors.Wrap(err, "unmarshal auth-session frame")
	}
	completionFrame, ok := frame.GetBody().(*api.WsAuthSessionServerFrame_Completion)
	if !ok || completionFrame.Completion == nil {
		return nil, "", "", errors.New("auth-session frame missing completion")
	}
	completion := completionFrame.Completion

	// 8. Decrypt with ephemeral privkey.
	plaintext, err := peer.DecryptWithPrivKey(ephPriv, session_handoff.EncryptContext, completion.GetEncryptedSessionKey())
	if err != nil {
		return nil, "", "", errors.Wrap(err, "decrypt session key")
	}
	defer scrub.Scrub(plaintext)

	// 9. Parse session privkey from PEM.
	sessionPrivKey, err := keypem.ParsePrivKeyPem(plaintext)
	if err != nil {
		return nil, "", "", errors.Wrap(err, "parse session privkey")
	}

	// 10. Send HandoffAck.
	ack := &session_handoff.HandoffAck{}
	ackData, err := ack.MarshalVT()
	if err != nil {
		return nil, "", "", errors.Wrap(err, "marshal handoff ack")
	}
	if err := conn.Write(ctx, websocket.MessageBinary, ackData); err != nil {
		return nil, "", "", errors.Wrap(err, "send handoff ack")
	}

	cleanupSession = func() {}
	return sessionPrivKey, completion.GetAccountId(), completion.GetEntityId(), nil
}

// getDeviceName returns the hostname or a fallback device name.
func getDeviceName() string {
	name, err := os.Hostname()
	if err != nil || name == "" {
		return "Desktop"
	}
	return name
}

// openBrowserValidated opens the default browser to the given URL after
// validating that the URL uses https, or http for loopback hosts only, and its
// host is in allowedHosts. On Windows it invokes rundll32's
// FileProtocolHandler directly rather than shelling through cmd /c start,
// which would re-parse the URL as command line input and allow argument
// injection.
func openBrowserValidated(rawURL string, allowedHosts []string) error {
	if err := validateOpenURL(rawURL, allowedHosts); err != nil {
		return err
	}
	return browserOpener(rawURL)
}

// validateOpenURL returns an error unless rawURL parses cleanly, uses https or
// loopback-only http, and has a host matching one of allowedHosts
// (case-insensitive, exact). An empty allowedHosts list skips the host check.
func validateOpenURL(rawURL string, allowedHosts []string) error {
	if rawURL == "" {
		return errors.New("empty browser url")
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return errors.Wrap(err, "parse browser url")
	}
	if u.Host == "" {
		return errors.New("browser url has no host")
	}
	if u.Scheme != "https" {
		if u.Scheme != "http" || !isLoopbackOpenHost(u.Hostname()) {
			return errors.Errorf(
				"browser url scheme must be https (or http for localhost), got %q",
				u.Scheme,
			)
		}
	}
	if len(allowedHosts) == 0 {
		return nil
	}
	host := strings.ToLower(u.Host)
	for _, allowed := range allowedHosts {
		if strings.ToLower(allowed) == host {
			return nil
		}
	}
	return errors.Errorf("browser url host %q is not in the allowlist", u.Host)
}

// isLoopbackOpenHost reports whether the browser-open target is a loopback host
// that is acceptable for local development.
func isLoopbackOpenHost(host string) bool {
	host = strings.ToLower(host)
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

// hostsFromURLs parses each URL and returns the list of hosts. Empty or
// unparseable entries are skipped.
func hostsFromURLs(urls ...string) []string {
	hosts := make([]string, 0, len(urls))
	for _, raw := range urls {
		if raw == "" {
			continue
		}
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			continue
		}
		hosts = append(hosts, u.Host)
	}
	return hosts
}

// browserOpener opens the default browser to the given URL without shell
// interpretation. The URL must already be validated by validateOpenURL.
var browserOpener = openBrowser

// openBrowser opens the default browser to the given URL without shell
// interpretation. The URL must already be validated by validateOpenURL.
func openBrowser(rawURL string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "--", rawURL).Start()
	case "linux":
		return exec.Command("xdg-open", rawURL).Start()
	case "windows":
		// rundll32 passes rawURL to url.dll's FileProtocolHandler as a single
		// argument; it does not shell-interpret the value. Avoid cmd /c start,
		// which treats the URL as command line input.
		return exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", rawURL).Start()
	default:
		return errors.Errorf("unsupported platform: %s", runtime.GOOS)
	}
}

func buildCreateSessionURL(apiEndpoint string) string {
	return strings.TrimRight(apiEndpoint, "/") + "/api/auth/session/create"
}

func buildHandoffBrowserURL(
	publicBaseURL string,
	payload string,
	authIntent string,
	username string,
) string {
	base := strings.TrimRight(publicBaseURL, "/") + "/#/auth/link/" + payload
	q := url.Values{}
	if authIntent != "" {
		q.Set("intent", authIntent)
	}
	if username != "" {
		q.Set("username", username)
	}
	qs := q.Encode()
	if qs == "" {
		return base
	}
	return base + "?" + qs
}

func buildHandoffWSURL(apiEndpoint string, wsTicket string) string {
	wsURL := strings.Replace(apiEndpoint, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	return strings.TrimRight(wsURL, "/") + "/api/auth/session/ws?tk=" + wsTicket
}
