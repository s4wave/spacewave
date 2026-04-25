package provider_spacewave

import (
	"context"
	"crypto/rand"
	"math/big"

	"github.com/pkg/errors"
	api "github.com/s4wave/spacewave/core/provider/spacewave/api"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// pairingCharset is the character set for pairing codes.
var pairingCharset = []byte("ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789")

// generatePairingCode generates an 8-character uppercase alphanumeric code.
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

// GeneratePairingCode generates an 8-char alphanumeric pairing code and
// registers it with the cloud provider via the authenticated session client.
// The relayURL and sessionPriv parameters are ignored (retained for interface
// compatibility with the local provider).
func (a *ProviderAccount) GeneratePairingCode(
	ctx context.Context,
	_ string,
	_ crypto.PrivKey,
	sessionPeerID peer.ID,
) (string, error) {
	cli := a.GetSessionClient()
	if cli == nil {
		return "", errors.New("session client not available")
	}

	code, err := generatePairingCode()
	if err != nil {
		return "", err
	}

	body, err := (&api.PairingRequest{
		Code:   code,
		PeerId: sessionPeerID.String(),
	}).MarshalVT()
	if err != nil {
		return "", errors.Wrap(err, "marshal pairing request")
	}

	if _, err := cli.doPost(ctx, "/api/pair", "application/octet-stream", body, nil, SeedReasonMutation); err != nil {
		return "", errors.Wrap(err, "post pairing code")
	}

	return code, nil
}

// CompletePairing looks up a pairing code to get the remote peer ID via the
// cloud provider endpoint. The sessionPriv and sessionPeerID params are unused
// (retained for interface compatibility with the local provider).
func (a *ProviderAccount) CompletePairing(
	ctx context.Context,
	_ string,
	code string,
	_ crypto.PrivKey,
	_ peer.ID,
) (peer.ID, error) {
	cli := a.GetSessionClient()
	if cli == nil {
		return "", errors.New("session client not available")
	}

	respBody, err := cli.doGet(ctx, "/api/pair/"+code, SeedReasonColdSeed)
	if err != nil {
		return "", errors.Wrap(err, "get pairing code")
	}

	var resp api.PairingResponse
	if err := resp.UnmarshalVT(respBody); err != nil {
		return "", errors.Wrap(err, "unmarshal pairing response")
	}

	remotePeerID, err := peer.IDB58Decode(resp.GetPeerId())
	if err != nil {
		return "", errors.Errorf("decode remote peer ID: %v", err)
	}

	return remotePeerID, nil
}
