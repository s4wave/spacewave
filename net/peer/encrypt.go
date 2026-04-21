package peer

import (
	"crypto/ed25519"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
)

// EncryptToPubKey encrypts a message to a public key.
//
// Supported types: Ed25519
// Context must be same when decrypting.
//
// Context should be globally unique, and application-specific.
// A good format for ctx strings is: [application] [commit timestamp] [purpose]
// e.g., "example.com 2019-12-25 16:18:03 session tokens v1"
// The purpose of these requirements is to ensure that an attacker cannot trick two different applications into using the same context string.
func EncryptToPubKey(pubKey crypto.PubKey, context string, msgSrc []byte) ([]byte, error) {
	spKey, err := crypto.PubKeyToStdKey(pubKey)
	if err != nil {
		return nil, err
	}

	switch t := spKey.(type) {
	case ed25519.PublicKey:
		return EncryptToEd25519(t, context, msgSrc)
	default:
		return nil, errors.Errorf("unhandled public key type: %s", pubKey.Type().String())
	}
}

// DecryptWithPrivKey decrypts with the given private key.
//
// Supported types: Ed25519
// Context must be same as when encrypting.
func DecryptWithPrivKey(privKey crypto.PrivKey, context string, ciphertext []byte) ([]byte, error) {
	spKey, err := crypto.PrivKeyToStdKey(privKey)
	if err != nil {
		return nil, err
	}

	switch t := spKey.(type) {
	case *ed25519.PrivateKey:
		return DecryptWithEd25519(*t, context, ciphertext)
	default:
		return nil, errors.Errorf("unhandled private key type: %s", privKey.Type().String())
	}
}
