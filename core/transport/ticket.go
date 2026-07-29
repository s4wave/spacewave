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

var errSignalTicketUnauthorized = errors.New("signal ticket unauthorized")

type signalTicketHTTPError struct {
	statusCode int
}

func (e *signalTicketHTTPError) Error() string {
	return "signal ticket: HTTP " + strconv.Itoa(e.statusCode)
}

func (e *signalTicketHTTPError) StatusCode() int {
	return e.statusCode
}

func (e *signalTicketHTTPError) Is(target error) bool {
	return e.statusCode == http.StatusUnauthorized && target == errSignalTicketUnauthorized
}

// signalURL joins a signaling endpoint path to its relative or absolute base.
func signalURL(baseURL, endpoint string) (*url.URL, error) {
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
// and returns a JWT token for WebSocket authentication.
func acquireSignalTicket(
	ctx context.Context,
	baseURL string,
	priv bifrost_crypto.PrivKey,
	pid peer.ID,
	envPfx string,
) (string, error) {
	ticketURL, err := signalURL(baseURL, "/api/signal/ticket")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ticketURL.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/octet-stream")

	if err := SignHTTPRequest(req, nil, priv, pid, envPfx); err != nil {
		return "", errors.Wrap(err, "sign signal ticket request")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "post signal ticket")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", &signalTicketHTTPError{statusCode: resp.StatusCode}
	}

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
