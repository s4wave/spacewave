package provider_local

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
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// pairingCharset is the character set for pairing codes (uppercase alphanumeric).
var pairingCharset = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// generatePairingCode generates an 8-character uppercase alphanumeric pairing code.
func generatePairingCode() (string, error) {
	n := big.NewInt(int64(len(pairingCharset)))
	code := make([]byte, 8)
	for i := range code {
		idx, err := rand.Int(rand.Reader, n)
		if err != nil {
			return "", errors.Wrap(err, "generate random index")
		}
		code[i] = pairingCharset[idx.Int64()]
	}
	return string(code), nil
}

// GeneratePairingCode generates an 8-char alphanumeric pairing code,
// registers it with the pairing relay via signed HTTP POST, and returns the code.
// Ensures the session transport is running before posting the code.
func (a *ProviderAccount) GeneratePairingCode(
	ctx context.Context,
	relayURL string,
	sessionPriv crypto.PrivKey,
	sessionPeerID peer.ID,
) (string, error) {
	if err := a.EnsureSessionTransport(ctx, sessionPriv, relayURL); err != nil {
		return "", errors.Wrap(err, "start session transport")
	}

	code, err := generatePairingCode()
	if err != nil {
		return "", err
	}

	body, err := (&api.PairingRequest{
		Code:   code,
		PeerId: sessionPeerID.String(),
	}).MarshalJSON()
	if err != nil {
		return "", errors.Wrap(err, "marshal pairing request")
	}

	reqURL, err := url.JoinPath(relayURL, "/pair")
	if err != nil {
		return "", errors.Wrap(err, "build pairing URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, bytes.NewReader(body))
	if err != nil {
		return "", errors.Wrap(err, "create pairing request")
	}
	req.Header.Set("Content-Type", "application/json")

	if err := transport.SignHTTPRequest(req, body, sessionPriv, sessionPeerID, ""); err != nil {
		return "", errors.Wrap(err, "sign pairing request")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "post pairing code")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return "", errors.New("pairing code conflict, retry with new code")
	}
	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return "", errors.Errorf("pairing relay returned %d: %s", resp.StatusCode, string(respBody))
	}

	a.SetPairingCode(code, sessionPriv)
	return code, nil
}

// CompletePairing looks up a pairing code to get the remote peer ID.
// Ensures the session transport is running and adds an EstablishLinkWithPeer
// directive for the remote peer on the transport's child bus.
func (a *ProviderAccount) CompletePairing(
	ctx context.Context,
	relayURL string,
	code string,
	sessionPriv crypto.PrivKey,
	sessionPeerID peer.ID,
) (peer.ID, error) {
	if err := a.EnsureSessionTransport(ctx, sessionPriv, relayURL); err != nil {
		return "", errors.Wrap(err, "start session transport")
	}

	reqURL, err := url.JoinPath(relayURL, "/pair", code)
	if err != nil {
		return "", errors.Wrap(err, "build pairing URL")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return "", errors.Wrap(err, "create pairing lookup request")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", errors.Wrap(err, "get pairing code")
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", errors.Wrap(err, "read pairing response")
	}

	if resp.StatusCode != http.StatusOK {
		return "", errors.Errorf("pairing relay returned %d: %s", resp.StatusCode, string(respBody))
	}

	var pr api.PairingResponse
	if err := pr.UnmarshalJSON(respBody); err != nil {
		return "", errors.Wrap(err, "unmarshal pairing response")
	}

	remotePeerID, err := peer.IDB58Decode(pr.GetPeerId())
	if err != nil {
		return "", errors.Errorf("decode remote peer ID: %v", err)
	}

	// Watch for bifrost link with the remote peer on the child bus.
	if err := a.SetPairingRemotePeer(remotePeerID, sessionPriv); err != nil {
		a.le.WithError(err).Warn("failed to set up link watch for remote peer")
	}

	return remotePeerID, nil
}
