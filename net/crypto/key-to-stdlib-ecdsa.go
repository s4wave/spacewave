//go:build !js

package crypto

import "crypto/ecdsa"

// ECDSAPublicKeyFromStdKey wraps a standard library *ecdsa.PublicKey. This is
// provided for interop with x509 certificates that may use ECDSA; bifrost does
// not generate ECDSA keys itself. The js browser build omits this adapter so the
// GoScript closure never imports crypto/ecdsa, which transitively pulls
// golang.org/x/crypto/cryptobyte and reflect into the bundle; browser peer
// identities are ed25519 and the ECDSA interop has no js caller.
func ECDSAPublicKeyFromStdKey(pub *ecdsa.PublicKey) PubKey {
	return &ecdsaPublicKeyAdapter{pub: pub}
}

// ecdsaPublicKeyAdapter wraps *ecdsa.PublicKey for verify-only use.
type ecdsaPublicKeyAdapter struct {
	pub *ecdsa.PublicKey
}

// Type returns the libp2p ECDSA key type.
func (k *ecdsaPublicKeyAdapter) Type() KeyType { return KeyType(3) }

// Raw returns ErrBadKeyType; raw ECDSA bytes are not supported.
func (k *ecdsaPublicKeyAdapter) Raw() ([]byte, error) {
	return nil, ErrBadKeyType
}

// Equals returns true if the other key is the same ECDSA public key.
func (k *ecdsaPublicKeyAdapter) Equals(o Key) bool {
	other, ok := o.(*ecdsaPublicKeyAdapter)
	if !ok {
		return false
	}
	return k.pub.Equal(other.pub)
}

// Verify checks an ASN.1-encoded ECDSA signature over data.
func (k *ecdsaPublicKeyAdapter) Verify(data []byte, sig []byte) (bool, error) {
	return ecdsa.VerifyASN1(k.pub, data, sig), nil
}
