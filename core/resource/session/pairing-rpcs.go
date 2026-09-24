package resource_session

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/pairing"
	provider "github.com/s4wave/spacewave/core/provider"
	provider_spacewave "github.com/s4wave/spacewave/core/provider/spacewave"
	"github.com/s4wave/spacewave/net/peer"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// getPairingEngine resolves pairing from the mounted Session that owns its key.
func (r *SessionResource) getPairingEngine() (*pairing.Engine, error) {
	supported, ok := r.session.(pairing.Session)
	if !ok {
		return nil, errors.New("this Session does not support pairing")
	}
	return supported.GetPairingEngine()
}

// getPairingRelay returns the relay endpoint and signing context that must be
// used as one contract; staging rejects prod-context signatures.
func (r *SessionResource) getPairingRelay(ctx context.Context) (pairing.Relay, error) {
	swProv, swProvRef, err := provider.ExLookupProvider(ctx, r.b, "spacewave", false, nil)
	if err != nil {
		return pairing.Relay{}, errors.Wrap(err, "lookup cloud provider for pairing relay")
	}
	if swProv == nil {
		return pairing.Relay{}, errors.New("no cloud provider configured for pairing relay")
	}
	defer swProvRef.Release()
	swp, ok := swProv.(*provider_spacewave.Provider)
	if !ok {
		return pairing.Relay{}, errors.New("unexpected spacewave provider type")
	}
	endpoint := swp.GetEndpoint()
	if endpoint == "" {
		return pairing.Relay{}, errors.New("cloud provider endpoint is empty")
	}
	return pairing.Relay{
		URL:              endpoint,
		SigningEnvPrefix: swp.GetSigningEnvPrefix(),
		Client:           swp.GetHTTPClient(),
	}, nil
}

// GeneratePairingCode creates an 8-char pairing code for P2P device linking.
func (r *SessionResource) GeneratePairingCode(ctx context.Context, _ *s4wave_session.GeneratePairingCodeRequest) (*s4wave_session.GeneratePairingCodeResponse, error) {
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	engine, err := r.getPairingEngine()
	if err != nil {
		return nil, err
	}

	relay, err := r.getPairingRelay(ctx)
	if err != nil {
		return nil, err
	}

	code, err := engine.GenerateCode(ctx, relay)
	if err != nil {
		return nil, err
	}

	return &s4wave_session.GeneratePairingCodeResponse{Code: code}, nil
}

// CompletePairing links a remote session to the peer that registered a
// pairing code, using the peer ID the page already resolved when present.
func (r *SessionResource) CompletePairing(ctx context.Context, req *s4wave_session.CompletePairingRequest) (*s4wave_session.CompletePairingResponse, error) {
	// Require an unlocked Session to sign the exchange.
	privKey := r.session.GetPrivKey()
	if privKey == nil {
		return nil, errors.New("session is locked")
	}

	// Resolve the pairing engine and relay for the mounted Session.
	engine, err := r.getPairingEngine()
	if err != nil {
		return nil, err
	}
	relay, err := r.getPairingRelay(ctx)
	if err != nil {
		return nil, err
	}

	// Use the peer the page resolved before boot, or resolve the code now.
	remotePeerID, _, err := peer.ParsePeerIDWithPubKey(req.GetRemotePeerId())
	if req.GetRemotePeerId() == "" {
		remotePeerID, err = pairing.ResolveCode(ctx, relay, req.GetCode())
	}
	if err != nil {
		return nil, err
	}

	// Link to the peer and hold the link through enrollment.
	if err := engine.CompletePeer(ctx, relay, remotePeerID, req.GetOfferCurrentAccount(), req.GetLabel()); err != nil {
		return nil, err
	}
	return &s4wave_session.CompletePairingResponse{RemotePeerId: remotePeerID.String()}, nil
}

// SelectPairingAccount fixes the account proposal before either client approves it.
func (r *SessionResource) SelectPairingAccount(ctx context.Context, req *s4wave_session.SelectPairingAccountRequest) (*s4wave_session.SelectPairingAccountResponse, error) {
	// Resolve the mounted Session's operation and submit the selected relationship.
	engine, err := r.getPairingEngine()
	if err != nil {
		return nil, err
	}
	if err := engine.SelectAccount(req.GetOutcome()); err != nil {
		return nil, err
	}
	return &s4wave_session.SelectPairingAccountResponse{}, nil
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

	emoji, err := pairing.DeriveSASEmoji(
		privKey, remotePub,
		r.session.GetPeerId(), remotePeerID,
	)
	if err != nil {
		return nil, err
	}

	return &s4wave_session.GetSASEmojiResponse{Emoji: emoji}, nil
}
