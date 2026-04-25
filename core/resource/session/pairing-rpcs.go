package resource_session

import (
	"context"

	"github.com/pkg/errors"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_local "github.com/s4wave/spacewave/core/provider/local"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// pairingProvider is the interface for provider accounts that support pairing.
type pairingProvider interface {
	GeneratePairingCode(ctx context.Context, relayURL string, sessionPriv crypto.PrivKey, sessionPeerID peer.ID) (string, error)
	CompletePairing(ctx context.Context, relayURL string, code string, sessionPriv crypto.PrivKey, sessionPeerID peer.ID) (peer.ID, error)
}

// getRelayURL resolves the pairing relay URL from the cloud provider endpoint.
// The spacewave provider ignores the relay URL (uses its session client), so
// this only matters for local provider sessions that need the cloud endpoint.
func (r *SessionResource) getRelayURL(ctx context.Context) (string, error) {
	if _, ok := r.session.GetProviderAccount().(*provider_spacewave.ProviderAccount); ok {
		return "", nil
	}
	swProv, swProvRef, err := provider.ExLookupProvider(ctx, r.b, "spacewave", false, nil)
	if err != nil {
		return "", errors.Wrap(err, "lookup cloud provider for pairing relay")
	}
	if swProv == nil {
		return "", errors.New("no cloud provider configured for pairing relay")
	}
	defer swProvRef.Release()
	swp, ok := swProv.(*provider_spacewave.Provider)
	if !ok {
		return "", errors.New("unexpected spacewave provider type")
	}
	endpoint := swp.GetEndpoint()
	if endpoint == "" {
		return "", errors.New("cloud provider endpoint is empty")
	}
	return endpoint, nil
}

// GeneratePairingCode creates an 8-char pairing code for P2P device linking.
func (r *SessionResource) GeneratePairingCode(ctx context.Context, _ *s4wave_session.GeneratePairingCodeRequest) (*s4wave_session.GeneratePairingCodeResponse, error) {
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	providerAcc := r.session.GetProviderAccount()
	pp, ok := providerAcc.(pairingProvider)
	if !ok {
		return nil, errors.New("provider does not support pairing")
	}

	relayURL, err := r.getRelayURL(ctx)
	if err != nil {
		return nil, err
	}

	code, err := pp.GeneratePairingCode(ctx, relayURL, privKey, r.session.GetPeerId())
	if err != nil {
		return nil, err
	}

	return &s4wave_session.GeneratePairingCodeResponse{Code: code}, nil
}

// CompletePairing resolves a pairing code to link a remote session.
func (r *SessionResource) CompletePairing(ctx context.Context, req *s4wave_session.CompletePairingRequest) (*s4wave_session.CompletePairingResponse, error) {
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	providerAcc := r.session.GetProviderAccount()
	pp, ok := providerAcc.(pairingProvider)
	if !ok {
		return nil, errors.New("provider does not support pairing")
	}

	relayURL, err := r.getRelayURL(ctx)
	if err != nil {
		return nil, err
	}

	remotePeerID, err := pp.CompletePairing(ctx, relayURL, req.GetCode(), privKey, r.session.GetPeerId())
	if err != nil {
		return nil, err
	}

	return &s4wave_session.CompletePairingResponse{RemotePeerId: remotePeerID.String()}, nil
}

// GetSASEmoji derives SAS emoji for verifying a P2P link with a remote peer.
func (r *SessionResource) GetSASEmoji(ctx context.Context, req *s4wave_session.GetSASEmojiRequest) (*s4wave_session.GetSASEmojiResponse, error) {
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	remotePeerID, err := peer.IDB58Decode(req.GetRemotePeerId())
	if err != nil {
		return nil, errors.Wrap(err, "decode remote peer ID")
	}

	remotePub, err := remotePeerID.ExtractPublicKey()
	if err != nil {
		return nil, errors.Wrap(err, "extract remote public key")
	}

	emoji, err := provider_local.DeriveSASEmoji(
		privKey, remotePub,
		r.session.GetPeerId(), remotePeerID,
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_session.GetSASEmojiResponse{Emoji: emoji}, nil
}
