package transport

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// errSignalTicketUnauthorized matches a ticket refusal the peer gives up on.
var errSignalTicketUnauthorized = errors.New("signal ticket unauthorized")

// signalTicketHTTPError is a ticket request the server answered without a
// ticket.
type signalTicketHTTPError struct {
	statusCode int
}

// Error reports the refusal's HTTP status.
func (e *signalTicketHTTPError) Error() string {
	return "signal ticket: HTTP " + strconv.Itoa(e.statusCode)
}

// StatusCode returns the refusal's HTTP status.
func (e *signalTicketHTTPError) StatusCode() int {
	return e.statusCode
}

// Is matches errSignalTicketUnauthorized for a 401 refusal.
func (e *signalTicketHTTPError) Is(target error) bool {
	return e.statusCode == http.StatusUnauthorized && target == errSignalTicketUnauthorized
}

// signalURL joins a signaling endpoint path to its relative or absolute base.
func signalURL(baseURL, endpoint string) (*url.URL, error) {
	// Resolve the signaling endpoint path against its configured base URL.
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	u.Path = path.Join(u.Path, endpoint)
	u.RawPath = ""
	return u, nil
}

// signalWebSocketURL builds a signaling WebSocket URL with an encoded ticket.
func signalWebSocketURL(baseURL, ticket string) (string, error) {
	// Build the WebSocket endpoint and encode its authorization ticket.
	u, err := signalURL(baseURL, "/api/signal/ws")
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	query := u.Query()
	query.Set("tk", ticket)
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// acquireSignalTicket POSTs to /signal/ticket with entity-signed headers
// and returns a JWT token for WebSocket authentication. It signs at clock's
// time and learns the server's time from each answer. A request refused while
// the clocks disagreed is signed again once, at the server's time.
func acquireSignalTicket(
	ctx context.Context,
	baseURL string,
	priv bifrost_crypto.PrivKey,
	pid peer.ID,
	envPfx string,
	clock *serverClock,
) (string, error) {
	// Construct the binary signal ticket request at the configured endpoint.
	ticketURL, err := signalURL(baseURL, "/api/signal/ticket")
	if err != nil {
		return "", err
	}
	resp, err := postSignalTicket(ctx, ticketURL.String(), priv, pid, envPfx, clock)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// A refusal that corrected the clock is signed again at the server's time.
	if resp.StatusCode == http.StatusForbidden && clock.observe(resp) {
		resp.Body.Close()
		resp, err = postSignalTicket(ctx, ticketURL.String(), priv, pid, envPfx, clock)
		if err != nil {
			return "", err
		}
		defer resp.Body.Close()
	}
	clock.observe(resp)

	// Require a successful HTTP response before decoding the signal ticket.
	if resp.StatusCode != http.StatusOK {
		return "", &signalTicketHTTPError{statusCode: resp.StatusCode}
	}

	// Decode the binary signal ticket response and require its token.
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read signal ticket response")
	}
	var result api.SignalTicketResponse
	if err := result.UnmarshalVT(data); err != nil {
		return "", errors.Wrap(err, "decode signal ticket response")
	}
	if result.GetToken() == "" {
		return "", errors.New("signal ticket: empty token")
	}

	return result.GetToken(), nil
}

// postSignalTicket sends one ticket request signed at clock's time.
func postSignalTicket(
	ctx context.Context,
	ticketURL string,
	priv bifrost_crypto.PrivKey,
	pid peer.ID,
	envPfx string,
	clock *serverClock,
) (*http.Response, error) {
	// Build the request for a binary ticket.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ticketURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/octet-stream")

	// Authenticate the ticket request with the session peer signature.
	if err := signHTTPRequestAt(req, nil, priv, pid, envPfx, clock.now()); err != nil {
		return nil, errors.Wrap(err, "sign signal ticket request")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "post signal ticket")
	}
	return resp, nil
}
