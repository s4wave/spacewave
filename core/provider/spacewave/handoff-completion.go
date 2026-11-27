package provider_spacewave

import (
	"bytes"
	"context"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	session_handoff "github.com/s4wave/spacewave/core/session/handoff"
	"github.com/s4wave/spacewave/net/httpclient"
)

// RelayHandoffCompletion delivers enrollment to the waiting client's auth session.
func (p *Provider) RelayHandoffCompletion(ctx context.Context, nonce string, completion *session_handoff.HandoffCompletion) error {
	// Encode the registered Session for the receiving client.
	completionData, err := completion.MarshalVT()
	if err != nil {
		return errors.Wrap(err, "marshal handoff completion")
	}

	// Build the relay request. The endpoint may be the serving origin "/",
	// so join rather than concatenate.
	completeURL, err := url.JoinPath(p.endpoint, "/api/auth/session", nonce, "complete")
	if err != nil {
		return errors.Wrap(err, "build completion URL")
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, completeURL, bytes.NewReader(completionData))
	if err != nil {
		return errors.Wrap(err, "build completion request")
	}
	httpReq.Header.Set("Content-Type", "application/octet-stream")

	// Relay the completion to the waiting client's auth session.
	httpResp, err := p.httpCli.Do(httpReq)
	if err != nil {
		return errors.Wrap(err, "relay handoff completion")
	}
	defer httpclient.DrainAndCloseResponseBody(httpResp)
	if httpResp.StatusCode != http.StatusOK {
		return errors.Errorf("handoff completion relay failed: %d", httpResp.StatusCode)
	}

	return nil
}

// RelayHandoffCompletion delivers account enrollment through the account's provider.
func (a *ProviderAccount) RelayHandoffCompletion(ctx context.Context, nonce string, completion *session_handoff.HandoffCompletion) error {
	return a.p.RelayHandoffCompletion(ctx, nonce, completion)
}
