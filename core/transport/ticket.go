package transport

import (
	"context"
	"io"
	"net/http"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// acquireSignalTicket POSTs to /signal/ticket with entity-signed headers
// and returns a JWT token for WebSocket authentication.
func acquireSignalTicket(
	ctx context.Context,
	baseURL string,
	priv bifrost_crypto.PrivKey,
	pid peer.ID,
	envPfx string,
) (string, error) {
	ticketURL := baseURL + "/api/signal/ticket"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ticketURL, nil)
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
		return "", errors.Errorf("signal ticket: HTTP %d", resp.StatusCode)
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
