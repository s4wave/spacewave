package provider_spacewave_handoff

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	websocket "github.com/aperturerobotics/go-websocket"
	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

// WaitForDesktopSSOLink opens the system browser to the provider authorize
// URL returned by the cloud and waits for the OAuth result on the auth-session
// WebSocket relay. The caller (an authenticated SessionClient) must first call
// POST /api/auth/sso/link/start to obtain wsTicket and openURL, then pass them
// here. wsTicket's JWT subject carries the auth-session nonce used for session
// cleanup and the exchange fallback.
//
// The relay payload is a DesktopSSOLinkResult containing {provider, code},
// which the SessionDetails UI feeds into the existing Account.LinkSSO path.
func WaitForDesktopSSOLink(
	ctx context.Context,
	httpCli *http.Client,
	endpoint string,
	provider string,
	wsTicket string,
	openURL string,
) (*api.DesktopSSOLinkResult, error) {
	if wsTicket == "" {
		return nil, errors.New("server did not return a desktop sso-link websocket ticket")
	}
	if openURL == "" {
		return nil, errors.New("server did not return a desktop sso-link browser url")
	}

	nonce, err := parseJWTSubject(wsTicket)
	if err != nil {
		return nil, errors.Wrap(err, "parse desktop sso-link nonce")
	}

	allowedHosts := ssoProviderHosts[provider]
	if len(allowedHosts) == 0 {
		return nil, errors.Errorf("unsupported sso provider %q", provider)
	}

	cleanupSession := func() {
		delCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		defer cancel()
		if err := deleteAuthSession(delCtx, httpCli, endpoint, nonce, wsTicket); err != nil {
			log.Println("warning: failed to delete SSO-link auth session:", err)
		}
	}
	defer func() {
		cleanupSession()
	}()

	wsURL := strings.Replace(endpoint, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1)
	wsURL += "/api/auth/session/ws?tk=" + wsTicket

	if openErr := openBrowserValidated(openURL, allowedHosts); openErr != nil {
		return nil, errors.Wrap(openErr, "open browser for SSO link")
	}

	// Wait for SSO-link result via WS, falling back to HTTP exchange on read
	// error. WS returns the legacy DesktopSSOLinkResult JSON shape; HTTP returns
	// the same proto in binary. Both converge into a single typed result.
	var result *api.DesktopSSOLinkResult
	for attempts := 0; ; attempts++ {
		conn, _, dialErr := websocket.Dial(ctx, wsURL, nil)
		if dialErr != nil {
			return nil, errors.Wrap(dialErr, "connect websocket")
		}

		_, msg, readErr := conn.Read(ctx)
		_ = conn.Close(websocket.StatusNormalClosure, "")
		if readErr == nil {
			var frame api.WsAuthSessionServerFrame
			if err := frame.UnmarshalVT(msg); err != nil {
				return nil, errors.Wrap(err, "parse SSO-link frame")
			}
			body, ok := frame.GetBody().(*api.WsAuthSessionServerFrame_SsoLink)
			if !ok || body.SsoLink == nil {
				return nil, errors.New("auth-session frame missing SSO-link result")
			}
			result = body.SsoLink
			break
		}
		if ctx.Err() != nil {
			return nil, errors.Wrap(readErr, "read SSO-link result")
		}
		exchangeCtx, cancel := context.WithTimeout(context.Background(), httpTimeout)
		exchanged, exchangeErr := exchangeAuthSessionLinkResult(
			exchangeCtx,
			httpCli,
			endpoint,
			nonce,
		)
		cancel()
		if exchangeErr == nil && exchanged != nil {
			result = exchanged
			break
		}
		if attempts >= 2 {
			if exchangeErr != nil {
				return nil, errors.Wrapf(
					readErr,
					"read SSO-link result after exchange fallback %v",
					exchangeErr,
				)
			}
			return nil, errors.Wrap(readErr, "read SSO-link result")
		}
		select {
		case <-ctx.Done():
			return nil, errors.Wrap(readErr, "read SSO-link result")
		case <-time.After(250 * time.Millisecond):
		}
	}

	if result.GetCode() == "" {
		return nil, errors.New("SSO-link relay returned empty code")
	}
	if result.GetProvider() == "" {
		result.Provider = provider
	}
	cleanupSession = func() {}
	return result, nil
}
