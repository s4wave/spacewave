package confparse

import (
	"errors"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
)

// ParsePublicKeyPEM parses the public key from a configuration.
// If there is no public key specified, returns nil, nil.
func ParsePublicKeyPEM(pubKeyDat []byte) (crypto.PubKey, error) {
	if len(pubKeyDat) == 0 {
		return nil, nil
	}

	key, err := keypem.ParsePubKeyPem(pubKeyDat)
	if err != nil {
		return nil, err
	}
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
	if len(privKeyDat) == 0 {
		return nil, nil
	}

	key, err := keypem.ParsePrivKeyPem(privKeyDat)
	if err != nil {
		return nil, err
	}

	if key == nil {
		return nil, errors.New("no pem data found")
	}

	return key, nil
}

// MarshalPrivateKeyPEM marshals the private key in pem format.
func MarshalPrivateKeyPEM(key crypto.PrivKey) ([]byte, error) {
	return keypem.MarshalPrivKeyPem(key)
}
