package provider_spacewave_handoff

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/pkg/errors"
	alpha_nethttp "github.com/s4wave/spacewave/core/nethttp"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
)

const httpTimeout = 5 * time.Second

func buildDeleteSessionURL(apiEndpoint string) string {
	return strings.TrimRight(apiEndpoint, "/") + "/api/auth/session/delete"
}

func buildSSOSignInExchangeURL(apiEndpoint string) string {
	return strings.TrimRight(apiEndpoint, "/") + "/api/auth/sso/result/exchange"
}

func buildSSOLinkExchangeURL(apiEndpoint string) string {
	return strings.TrimRight(apiEndpoint, "/") + "/api/auth/sso/link/result/exchange"
}

func deleteAuthSession(
	ctx context.Context,
	httpCli *http.Client,
	apiEndpoint string,
	nonce string,
	wsTicket string,
) error {
	reqBody, err := (&api.AuthSessionDeleteRequest{
		Nonce:    nonce,
		WsTicket: wsTicket,
	}).MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal auth session delete request")
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		buildDeleteSessionURL(apiEndpoint),
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return errors.Wrap(err, "build auth session delete request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpResp, err := httpCli.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "delete auth session")
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(httpResp)
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return errors.Wrap(err, "read auth session delete response")
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return errors.Errorf(
			"delete auth session failed: %d: %s",
			httpResp.StatusCode,
			string(respBody),
		)
	}
	return nil
}

// exchangeAuthSessionSignInResult exchanges a sign-in flow auth-session nonce
// for the stored OAuth result via POST /api/auth/sso/result/exchange.
// Returns nil if the cloud reports 404 (no result stored yet).
// Per OQ-8, the desktop SSO-link flow uses exchangeAuthSessionLinkResult
// against a different endpoint.
func exchangeAuthSessionSignInResult(
	ctx context.Context,
	httpCli *http.Client,
	apiEndpoint string,
	nonce string,
) (*api.SSOCodeExchangeResponse, error) {
	body, err := exchangeAuthSessionResultBody(
		ctx,
		httpCli,
		buildSSOSignInExchangeURL(apiEndpoint),
		nonce,
	)
	if err != nil || body == nil {
		return nil, err
	}
	var result api.SSOCodeExchangeResponse
	if err := result.UnmarshalVT(body); err != nil {
		return nil, errors.Wrap(err, "parse sso sign-in exchange response")
	}
	return &result, nil
}

// exchangeAuthSessionLinkResult exchanges a desktop SSO-link flow
// auth-session nonce for the stored relay result via POST
// /api/auth/sso/link/result/exchange. Returns nil if 404.
func exchangeAuthSessionLinkResult(
	ctx context.Context,
	httpCli *http.Client,
	apiEndpoint string,
	nonce string,
) (*api.DesktopSSOLinkResult, error) {
	body, err := exchangeAuthSessionResultBody(
		ctx,
		httpCli,
		buildSSOLinkExchangeURL(apiEndpoint),
		nonce,
	)
	if err != nil || body == nil {
		return nil, err
	}
	var result api.DesktopSSOLinkResult
	if err := result.UnmarshalVT(body); err != nil {
		return nil, errors.Wrap(err, "parse sso link exchange response")
	}
	return &result, nil
}

func exchangeAuthSessionResultBody(
	ctx context.Context,
	httpCli *http.Client,
	exchangeURL string,
	nonce string,
) ([]byte, error) {
	reqBody, err := (&api.AuthSessionResultExchangeRequest{Nonce: nonce}).MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "marshal auth session exchange request")
	}
	httpReq, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		exchangeURL,
		bytes.NewReader(reqBody),
	)
	if err != nil {
		return nil, errors.Wrap(err, "build auth session exchange request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")
	httpReq.Header.Set("Accept", "application/octet-stream")
	httpResp, err := httpCli.Do(httpReq)
	if err != nil {
		return nil, errors.Wrap(err, "exchange auth session result")
	}
	defer alpha_nethttp.DrainAndCloseResponseBody(httpResp)
	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "read auth session exchange response")
	}
	if httpResp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if httpResp.StatusCode < 200 || httpResp.StatusCode >= 300 {
		return nil, errors.Errorf(
			"exchange auth session result failed: %d: %s",
			httpResp.StatusCode,
			string(respBody),
		)
	}
	return respBody, nil
}
