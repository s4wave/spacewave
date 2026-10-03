package confparse

import (
	"errors"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// ParsePublicKeyPEM parses the public key from a configuration.
// If there is no public key specified, returns nil, nil.
func ParsePublicKeyPEM(pubKeyDat []byte) (crypto.PubKey, error) {
	// Treat an empty PEM public-key setting as an absent key.
	if len(pubKeyDat) == 0 {
		return nil, nil
	}

	// Parse the configured public key from its PEM data.
	key, err := keypem.ParsePubKeyPem(pubKeyDat)
	if err != nil {
		return nil, err
	}

	// Require a public key in the nonempty PEM data.
	// note: field has non-zero length but is not a valid key.
	if key == nil {
		return nil, errors.New("no pem data found")
	}

	return key, nil
}

// MarshalPublicKeyPEM marshals the public key in pem format.
func MarshalPublicKeyPEM(key crypto.PubKey) ([]byte, error) {
	return keypem.MarshalPubKeyPem(key)
}

// ParsePrivateKeyPEM parses the private key from a configuration.
// If there is no private key specified, returns nil, nil.
func ParsePrivateKeyPEM(privKeyDat []byte) (crypto.PrivKey, error) {
	// Treat an empty PEM private-key setting as an absent key.
	if len(privKeyDat) == 0 {
		return nil, nil
	}

	// Parse the configured private key from its PEM data.
	key, err := keypem.ParsePrivKeyPem(privKeyDat)
	if err != nil {
		return nil, err
	}

	// Require a private key in the nonempty PEM data.
	if key == nil {
		return nil, errors.New("no pem data found")
	}

	return key, nil
}

// MarshalPrivateKeyPEM marshals the private key in pem format.
func MarshalPrivateKeyPEM(key crypto.PrivKey) ([]byte, error) {
	return keypem.MarshalPrivKeyPem(key)
}
