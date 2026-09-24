package pairing

import (
	"bytes"
	"context"
	"crypto/rand"
	"io"
	"math/big"
	"net/http"
	"net/url"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/httpclient"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
)

// Relay supplies the configured code and signaling service as one signing scope.
type Relay struct {
	// URL is the base URL that serves the /api/pair routes.
	URL string
	// SigningEnvPrefix scopes request signatures to the relay's environment.
	SigningEnvPrefix string
	// Client performs relay requests; nil uses http.DefaultClient.
	Client *http.Client
}

// client returns the HTTP client for relay requests.
func (r Relay) client() *http.Client {
	if r.Client != nil {
		return r.Client
	}
	return http.DefaultClient
}

// GenerateCode publishes a code after the Session transport is ready to accept
// an authenticated peer. Local and cloud accounts use the same relay contract.
func (e *Engine) GenerateCode(ctx context.Context, relay Relay) (string, error) {
	// Replace any active attempt and ready the Session transport.
	e.Clear()
	st, err := e.transport(ctx, relay)
	if err != nil {
		return "", err
	}

	// Draw the code from uppercase letters and digits.
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	code := make([]byte, 8)
	for i := range code {
		index, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", err
		}
		code[i] = alphabet[index.Int64()]
	}

	// Register the code with a request signed by this Session peer.
	body, err := (&api.PairingRequest{Code: string(code), PeerId: e.peerID.String()}).MarshalVT()
	if err != nil {
		return "", err
	}
	endpoint, err := url.JoinPath(relay.URL, "/api/pair")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/octet-stream")
	if err := transport.SignHTTPRequest(req, body, e.key, e.peerID, relay.SigningEnvPrefix); err != nil {
		return "", err
	}
	resp, err := relay.client().Do(req)
	if err != nil {
		return "", errors.Wrap(err, "register pairing code")
	}
	defer httpclient.DrainAndCloseResponseBody(resp)
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("pairing code registration failed: HTTP %d", resp.StatusCode)
	}

	// Wait for the peer that resolves the code.
	parentCtx, active := e.begin(true, true, string(code), "", StatusCodeGenerated)
	go e.runSolicit(parentCtx, active, st)
	return string(code), nil
}

// CompletePeer links to the peer that registered a code and retains the link
// through enrollment. The caller resolved the code, so the relay is used only
// for signaling.
func (e *Engine) CompletePeer(ctx context.Context, relay Relay, remotePeer peer.ID, offerCurrent bool) error {
	// Replace any active attempt and ready the Session transport.
	e.Clear()
	st, err := e.transport(ctx, relay)
	if err != nil {
		return err
	}

	// Hold the link in the background until the exchange ends.
	parentCtx, active := e.begin(false, offerCurrent, "", remotePeer, StatusWaitingForPeer)
	go func() {
		_, release, err := link.EstablishLinkWithPeerEx(parentCtx, st.GetChildBus(), e.peerID, remotePeer, false)
		if err != nil {
			e.fail(active, StatusFailed, err)
			return
		}
		defer release()
		e.update(active, func(a *attempt) { a.snapshot.Status = StatusPeerConnected })
		e.runSolicit(parentCtx, active, st)
	}()
	return nil
}

// ResolveCode consumes a code at the relay and returns the peer that
// registered it. The relay deletes the code, so a code resolves once.
func ResolveCode(ctx context.Context, relay Relay, code string) (peer.ID, error) {
	// Fetch the registration for the code.
	endpoint, err := url.JoinPath(relay.URL, "/api/pair", code)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	resp, err := relay.client().Do(req)
	if err != nil {
		return "", errors.Wrap(err, "resolve pairing code")
	}
	defer httpclient.DrainAndCloseResponseBody(resp)
	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("pairing code lookup failed: HTTP %d", resp.StatusCode)
	}

	// Parse the registering peer from the response.
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	response := &api.PairingResponse{}
	if err := response.UnmarshalVT(body); err != nil {
		return "", err
	}
	remotePeer, _, err := peer.ParsePeerIDWithPubKey(response.GetPeerId())
	return remotePeer, err
}
