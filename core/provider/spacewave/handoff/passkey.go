package provider_spacewave_handoff

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// StartPasskeyHandoff initiates desktop passkey by opening the system browser
// to the passkey ceremony URL and waiting for the result via WebSocket relay.
//
// The cloud-returned openUrl must use the configured accountEndpoint host; the
// launcher does not shell-interpret the URL.
func StartPasskeyHandoff(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	accountEndpoint string,
) (*api.DesktopPasskeyRelayResult, error) {
	startResp, err := startDesktopPasskey(ctx, httpCli, endpoint)
	if err != nil {
		return nil, err
	}
	return waitForDesktopPasskeyResult(
		ctx,
		httpCli,
		endpoint,
		accountEndpoint,
		startResp.GetNonce(),
		startResp.GetWsTicket(),
		startResp.GetOpenUrl(),
	)
}

// WaitForDesktopPasskeyRegister opens the system browser to the returned
// register ceremony URL, waits on the auth-session WebSocket, and returns the
// browser-collected register artifacts for the mounted account flow.
func WaitForDesktopPasskeyRegister(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	accountEndpoint string,
	nonce string,
	wsTicket string,
	openURL string,
) (*api.DesktopPasskeyRegisterResult, error) {
	result, err := waitForDesktopPasskeyRegisterRelay(
		ctx,
		httpCli,
		endpoint,
		accountEndpoint,
		nonce,
		wsTicket,
		openURL,
	)
	if err != nil {
		return nil, err
	}
	register := result.GetRegister()
	if register == nil {
		return nil, errors.New("desktop passkey register relay returned no register payload")
	}
	return register, nil
}

// WaitForDesktopPasskeyReauth opens the system browser to the returned reauth
// ceremony URL, waits on the auth-session WebSocket, and returns the
// browser-collected reauth artifacts for the existing unlock path.
func WaitForDesktopPasskeyReauth(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	accountEndpoint string,
	nonce string,
	wsTicket string,
	openURL string,
) (*api.DesktopPasskeyReauthResult, error) {
	result, err := waitForDesktopPasskeyReauthRelay(
		ctx,
		httpCli,
		endpoint,
		accountEndpoint,
		nonce,
		wsTicket,
		openURL,
	)
	if err != nil {
		return nil, err
	}
	reauth := result.GetReauth()
	if reauth == nil {
		return nil, errors.New("desktop passkey reauth relay returned no reauth payload")
	}
	return reauth, nil
}

// validatePasskeyOpenURL validates the passkey ceremony URL against the
// configured account endpoint host.
func validatePasskeyOpenURL(openURL string, accountEndpoint string) error {
	accountURL, err := url.Parse(accountEndpoint)
	if err != nil || accountURL.Host == "" {
		return errors.New("desktop passkey requires a valid account_endpoint")
	}
	openParsed, err := url.Parse(openURL)
	if err != nil {
		return errors.Wrap(err, "parse browser url")
	}
	if openParsed.Host == "" {
		return errors.New("browser url has no host")
	}
	if !strings.EqualFold(openParsed.Host, accountURL.Host) {
		return errors.Errorf(
			"desktop passkey open_url must use account_endpoint host %q",
			accountURL.Host,
		)
	}
	if openParsed.Scheme == "https" {
		return nil
	}
	if openParsed.Scheme == "http" &&
		strings.EqualFold(accountURL.Scheme, "http") &&
		strings.EqualFold(accountURL.Hostname(), "localhost") &&
		strings.EqualFold(openParsed.Hostname(), "localhost") {
		return nil
	}
	return errors.Errorf("browser url scheme must be https, got %q", openParsed.Scheme)
}

func startDesktopPasskey(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
) (*api.DesktopPasskeyStartResponse, error) {
	startReq := &api.DesktopPasskeyStartRequest{}
	startBody, err := startReq.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal desktop passkey start request")
	}
	startURL := strings.TrimRight(endpoint, "/") + "/api/auth/passkey/start"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, startURL, bytes.NewReader(startBody))
	if err != nil {
		return nil, errors.Wrap(err, "build desktop passkey start request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("Accept", "application/octet-stream")
	httpResp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "start desktop passkey")
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(httpResp)
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read desktop passkey start response")
	}
	if httpResp.StatusCode != http.StatusOK {
		return nil, errors.Errorf("desktop passkey start failed: %d: %s", httpResp.StatusCode, string(respBody))
	}

	var startResp api.DesktopPasskeyStartResponse
	if err := startResp.UnmarshalVT(respBody); err != nil {
		return nil, errors.Wrap(err, "parse desktop passkey start response")
	}
	return &startResp, nil
}

func waitForDesktopPasskeyResult(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	accountEndpoint string,
	nonce string,
	wsTicket string,
	openURL string,
) (*api.DesktopPasskeyRelayResult, error) {
	if nonce == "" {
		return nil, errors.New("server did not return a desktop passkey nonce")
	}
	if wsTicket == "" {
		return nil, errors.New("server did not return a desktop passkey websocket ticket")
	}
	if openURL == "" {
		return nil, errors.New("server did not return a desktop passkey browser url")
	}

	cleanupSession := func() {
		delCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		if err := deleteAuthSession(delCtx, httpCli, endpoint, nonce, wsTicket); err != nil {
			log.Println("warning: failed to delete passkey auth session:", err)
		}
	}
	defer func() {
		cleanupSession()
	}()

	wsURL := strings.Replace(endpoint, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/auth/session/ws?tk=" + wsTicket
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "connect websocket")
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if openErr := validatePasskeyOpenURL(openURL, accountEndpoint); openErr != nil {
		return nil, errors.Wrap(openErr, "open browser for passkey")
	}
	if openErr := browserOpener(openURL); openErr != nil {
		return nil, errors.Wrap(openErr, "open browser for passkey")
	}

	_, msg, err := conn.Read(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read desktop passkey result")
	}

	var frame api.WsAuthSessionServerFrame
	if err := frame.UnmarshalVT(msg); err != nil {
		return nil, errors.Wrap(err, "parse desktop passkey frame")
	}
	body, ok := frame.GetBody().(*api.WsAuthSessionServerFrame_PasskeyRelay)
	if !ok || body.PasskeyRelay == nil {
		return nil, errors.New("auth-session frame missing passkey relay")
	}
	relay, ok := body.PasskeyRelay.GetRelay().(*api.PasskeyRelay_RelayResult)
	if !ok || relay.RelayResult == nil {
		return nil, errors.New("auth-session frame missing passkey login relay")
	}
	cleanupSession = func() {}
	return relay.RelayResult, nil
}

func waitForDesktopPasskeyRegisterRelay(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	accountEndpoint string,
	nonce string,
	wsTicket string,
	openURL string,
) (*api.DesktopPasskeyRegisterRelayResult, error) {
	if nonce == "" {
		return nil, errors.New("server did not return a desktop passkey nonce")
	}
	if wsTicket == "" {
		return nil, errors.New("server did not return a desktop passkey websocket ticket")
	}
	if openURL == "" {
		return nil, errors.New("server did not return a desktop passkey browser url")
	}

	cleanupSession := func() {
		delCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		if err := deleteAuthSession(delCtx, httpCli, endpoint, nonce, wsTicket); err != nil {
			log.Println("warning: failed to delete passkey auth session:", err)
		}
	}
	defer func() {
		cleanupSession()
	}()

	wsURL := strings.Replace(endpoint, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/auth/session/ws?tk=" + wsTicket
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "connect websocket")
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if openErr := validatePasskeyOpenURL(openURL, accountEndpoint); openErr != nil {
		return nil, errors.Wrap(openErr, "open browser for passkey register")
	}
	if openErr := browserOpener(openURL); openErr != nil {
		return nil, errors.Wrap(openErr, "open browser for passkey register")
	}

	_, msg, err := conn.Read(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read desktop passkey register result")
	}

	var frame api.WsAuthSessionServerFrame
	if err := frame.UnmarshalVT(msg); err != nil {
		return nil, errors.Wrap(err, "parse desktop passkey register frame")
	}
	body, ok := frame.GetBody().(*api.WsAuthSessionServerFrame_PasskeyRelay)
	if !ok || body.PasskeyRelay == nil {
		return nil, errors.New("auth-session frame missing passkey relay")
	}
	relay, ok := body.PasskeyRelay.GetRelay().(*api.PasskeyRelay_RegisterRelay)
	if !ok || relay.RegisterRelay == nil {
		return nil, errors.New("auth-session frame missing passkey register relay")
	}
	cleanupSession = func() {}
	return relay.RegisterRelay, nil
}

func waitForDesktopPasskeyReauthRelay(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	accountEndpoint string,
	nonce string,
	wsTicket string,
	openURL string,
) (*api.DesktopPasskeyReauthRelayResult, error) {
	if nonce == "" {
		return nil, errors.New("server did not return a desktop passkey nonce")
	}
	if wsTicket == "" {
		return nil, errors.New("server did not return a desktop passkey websocket ticket")
	}
	if openURL == "" {
		return nil, errors.New("server did not return a desktop passkey browser url")
	}

	cleanupSession := func() {
		delCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		if err := deleteAuthSession(delCtx, httpCli, endpoint, nonce, wsTicket); err != nil {
			log.Println("warning: failed to delete passkey auth session:", err)
		}
	}
	defer func() {
		cleanupSession()
	}()

	wsURL := strings.Replace(endpoint, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/auth/session/ws?tk=" + wsTicket
	conn, _, err := websocket.Dial(ctx, wsURL, nil)
	if err != nil {
		return nil, errors.Wrap(err, "connect websocket")
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	if openErr := validatePasskeyOpenURL(openURL, accountEndpoint); openErr != nil {
		return nil, errors.Wrap(openErr, "open browser for passkey reauth")
	}
	if openErr := browserOpener(openURL); openErr != nil {
		return nil, errors.Wrap(openErr, "open browser for passkey reauth")
	}

	_, msg, err := conn.Read(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "read desktop passkey reauth result")
	}

	var frame api.WsAuthSessionServerFrame
	if err := frame.UnmarshalVT(msg); err != nil {
		return nil, errors.Wrap(err, "parse desktop passkey reauth frame")
	}
	body, ok := frame.GetBody().(*api.WsAuthSessionServerFrame_PasskeyRelay)
	if !ok || body.PasskeyRelay == nil {
		return nil, errors.New("auth-session frame missing passkey relay")
	}
	relay, ok := body.PasskeyRelay.GetRelay().(*api.PasskeyRelay_ReauthRelay)
	if !ok || relay.ReauthRelay == nil {
		return nil, errors.New("auth-session frame missing passkey reauth relay")
	}
	cleanupSession = func() {}
	return relay.ReauthRelay, nil
}
