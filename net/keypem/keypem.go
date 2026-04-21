package keypem

import (
	"encoding/pem"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// PrivPemType is the expected header type on private keys.
const PrivPemType = "LIBP2P PRIVATE KEY"

// PubPemType is the expected header type on public keys.
const PubPemType = "LIBP2P PUBLIC KEY"

// ParseKeyPem parses a private or public key in pem format.
// Derives the public key from the private key.
// Returns nil for the private key if not set.
// Returns nil, nil, nil if nothing found in the pem.
func ParseKeyPem(pemDat []byte) (crypto.PrivKey, crypto.PubKey, error) {
	b, _ := pem.Decode(pemDat)
	if b == nil {
		return nil, nil, nil
	}

	switch b.Type {
	case PrivPemType:
		pkey, err := crypto.UnmarshalPrivateKey(b.Bytes)
		if err != nil {
			return nil, nil, err
		}
		return pkey, pkey.GetPublic(), nil
	case PubPemType:
		pkey, err := crypto.UnmarshalPublicKey(b.Bytes)
		if err != nil {
			return nil, nil, err
		}
		return nil, pkey, nil
	default:
		return nil, nil, errors.Wrap(ErrUnexpectedPemType, b.Type)
	}
}

// ParsePrivKeyPem parses a private key in pem format.
// If none is found returns nil
func ParsePrivKeyPem(pemDat []byte) (crypto.PrivKey, error) {
	b, _ := pem.Decode(pemDat)
	if b == nil {
		return nil, nil
	}

	if b.Type != PrivPemType {
		return nil, errors.Wrap(ErrUnexpectedPemType, b.Type)
	}

	return crypto.UnmarshalPrivateKey(b.Bytes)
}

// MarshalPrivKeyPem marshals a private key to pem.
func MarshalPrivKeyPem(key crypto.PrivKey) ([]byte, error) {
	dat, err := crypto.MarshalPrivateKey(key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  PrivPemType,
		Bytes: dat,
	}), nil
}

// ParsePubKeyPem parses a public key in pem format.
// Accepts either a private key or a public key.
// If none is found returns nil
func ParsePubKeyPem(pemDat []byte) (crypto.PubKey, error) {
	_, pub, err := ParseKeyPem(pemDat)
	if err != nil {
		return nil, err
	}
	return pub, nil
}

// MarshalPubKeyPem marshals a public key to pem.
func MarshalPubKeyPem(key crypto.PubKey) ([]byte, error) {
	dat, err := crypto.MarshalPublicKey(key)
	if err != nil {
		return nil, err
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  PubPemType,
		Bytes: dat,
	}), nil
}
