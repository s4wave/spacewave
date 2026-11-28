// Package peer identifies a network identity by its public key.
//
// An ID is the IDENTITY multihash of a marshaled public key, so the key itself
// is recoverable from the ID (ExtractPublicKey) and no lookup is needed to check
// a signature. MatchesPublicKey and MatchesPrivateKey compare a key against an
// ID. The string form is base58 (IDB58Encode, IDB58Decode). A Peer is an ID
// with its public key and, when held, its private key. GetPrivKey returns
// ErrNoPrivKey for a peer known only by its public key.
//
// The GetPeer directive asks the bus for a local Peer, optionally by ID. The
// directive lets a transport or a session find the private key to sign with.
// Signature and SignedMsg sign data under a peer's key, and DeriveKey derives
// a secret from a private key under a caller-chosen context string, which must
// be globally unique to the application and purpose.
package peer

import (
	"context"
	"crypto/rand"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// Peer is the common interface for a keypair-based identity.
type Peer interface {
	// GetPeerID returns the peer ID.
	GetPeerID() ID

	// GetPubKey returns the public key of the peer.
	GetPubKey() crypto.PubKey

	// GetPrivKey returns the private key.
	// GetPrivKey may require an extra lookup.
	// Returns ErrNoPrivKey if the private key is unavailable.
	GetPrivKey(ctx context.Context) (crypto.PrivKey, error)
}

// NewPeer builds a new Peer object with a private key.
// If privKey is nil, one will be generated.
func NewPeer(privKey crypto.PrivKey) (Peer, error) {
	// Generate an Ed25519 key when the caller did not supply one.
	if privKey == nil {
		var err error
		privKey, _, err = crypto.GenerateEd25519Key(rand.Reader)
		if err != nil {
			return nil, err
		}
	}

	// Derive the peer identity from the private key.
	id, err := IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	// Assemble the in-memory peer with both key views.
	return &peer{
		privKey: privKey,
		pubKey:  privKey.GetPublic(),
		peerID:  id,
	}, nil
}

// NewPeerWithGenerateED25519 generates an ED25519 key and returns it + the peer.
func NewPeerWithGenerateED25519() (Peer, crypto.PrivKey, crypto.PubKey, error) {
	// Generate the keypair used by the new peer.
	privKey, pubKey, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, nil, nil, err
	}

	// Build the peer from the generated private key.
	p, err := NewPeer(privKey)
	if err != nil {
		return nil, nil, nil, err
	}

	// Return the peer and generated keypair.
	return p, privKey, pubKey, nil
}

// NewPeerWithPubKey builds a Peer with a public key.
func NewPeerWithPubKey(pubKey crypto.PubKey) (Peer, error) {
	// Derive the peer identity from the public key.
	id, err := IDFromPublicKey(pubKey)
	if err != nil {
		return nil, err
	}

	// Assemble a peer that exposes only the public key.
	return &peer{
		pubKey: pubKey,
		peerID: id,
	}, nil
}

// NewPeerWithID constructs a new Peer by extracting the pubkey from the ID.
func NewPeerWithID(id ID) (Peer, error) {
	pubKey, err := id.ExtractPublicKey()
	if err != nil {
		return nil, err
	}
	return NewPeerWithPubKey(pubKey)
}

// ParsePeerIDWithPubKey parses the peer ID and extracts the public key.
// Errors identify whether decoding the peer ID or extracting its key failed.
func ParsePeerIDWithPubKey(peerIDStr string) (ID, crypto.PubKey, error) {
	// Decode the peer identity from its base58 representation.
	peerID, err := IDB58Decode(peerIDStr)
	if err != nil {
		return "", nil, errors.Wrap(err, "decode peer ID")
	}

	// Recover the public key embedded in the decoded peer identity.
	peerPub, err := peerID.ExtractPublicKey()
	if err != nil {
		return peerID, nil, errors.Wrap(err, "extract public key")
	}
	return peerID, peerPub, nil
}

// peer implements Peer with an in-memory struct.
type peer struct {
	privKey crypto.PrivKey
	pubKey  crypto.PubKey
	peerID  ID
}

// GetPeerID returns the peer ID.
func (p *peer) GetPeerID() ID {
	return p.peerID
}

// GetPubKey returns the public key of the peer.
func (p *peer) GetPubKey() crypto.PubKey {
	return p.pubKey
}

// GetPrivKey returns the private key.
// Returns ErrNoPrivKey if the private key is unavailable.
func (p *peer) GetPrivKey(ctx context.Context) (crypto.PrivKey, error) {
	if p.privKey == nil {
		return nil, ErrNoPrivKey
	}
	return p.privKey, nil
}

// _ is a type assertion
var _ Peer = (*peer)(nil)
