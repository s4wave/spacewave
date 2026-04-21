package crypto

import (
	"bytes"
	"crypto/ed25519"
	"crypto/subtle"
	"io"

	"github.com/pkg/errors"
)

// Ed25519PrivateKey is an ed25519 private key.
type Ed25519PrivateKey struct {
	k ed25519.PrivateKey
}

// Ed25519PublicKey is an ed25519 public key.
type Ed25519PublicKey struct {
	k ed25519.PublicKey
}

// GenerateEd25519Key generates a new ed25519 private and public key pair.
func GenerateEd25519Key(src io.Reader) (PrivKey, PubKey, error) {
	pub, priv, err := ed25519.GenerateKey(src)
	if err != nil {
		return nil, nil, err
	}
	return &Ed25519PrivateKey{k: priv}, &Ed25519PublicKey{k: pub}, nil
}

// Type returns the key type (Ed25519).
func (k *Ed25519PrivateKey) Type() KeyType {
	return KeyType_Ed25519
}

// Raw returns the raw private key bytes.
func (k *Ed25519PrivateKey) Raw() ([]byte, error) {
	buf := make([]byte, len(k.k))
	copy(buf, k.k)
	return buf, nil
}

// Equals compares two ed25519 private keys.
func (k *Ed25519PrivateKey) Equals(o Key) bool {
	edk, ok := o.(*Ed25519PrivateKey)
	if !ok {
		return basicEquals(k, o)
	}
	return subtle.ConstantTimeCompare(k.k, edk.k) == 1
}

// GetPublic returns an ed25519 public key from a private key.
func (k *Ed25519PrivateKey) GetPublic() PubKey {
	return &Ed25519PublicKey{k: ed25519.PublicKey(k.k[ed25519.PrivateKeySize-ed25519.PublicKeySize:])}
}

// Sign returns a signature from an input message.
func (k *Ed25519PrivateKey) Sign(msg []byte) ([]byte, error) {
	return ed25519.Sign(k.k, msg), nil
}

// GetStdKey returns the standard library ed25519.PrivateKey.
func (k *Ed25519PrivateKey) GetStdKey() ed25519.PrivateKey {
	return k.k
}

// Type returns the key type (Ed25519).
func (k *Ed25519PublicKey) Type() KeyType {
	return KeyType_Ed25519
}

// Raw returns the raw public key bytes.
func (k *Ed25519PublicKey) Raw() ([]byte, error) {
	return k.k, nil
}

// Equals compares two ed25519 public keys.
func (k *Ed25519PublicKey) Equals(o Key) bool {
	edk, ok := o.(*Ed25519PublicKey)
	if !ok {
		return basicEquals(k, o)
	}
	return bytes.Equal(k.k, edk.k)
}

// Verify checks a signature against the input data.
func (k *Ed25519PublicKey) Verify(data []byte, sig []byte) (bool, error) {
	return ed25519.Verify(k.k, data, sig), nil
}

// GetStdKey returns the standard library ed25519.PublicKey.
func (k *Ed25519PublicKey) GetStdKey() ed25519.PublicKey {
	return k.k
}

// UnmarshalEd25519PublicKey returns a public key from input bytes.
func UnmarshalEd25519PublicKey(data []byte) (PubKey, error) {
	if len(data) != 32 {
		return nil, errors.New("expect ed25519 public key data size to be 32")
	}
	return &Ed25519PublicKey{k: ed25519.PublicKey(data)}, nil
}

// UnmarshalEd25519PrivateKey returns a private key from input bytes.
func UnmarshalEd25519PrivateKey(data []byte) (PrivKey, error) {
	switch len(data) {
	case ed25519.PrivateKeySize + ed25519.PublicKeySize:
		// Remove the redundant public key. See go-libp2p issue #36.
		redundantPk := data[ed25519.PrivateKeySize:]
		pk := data[ed25519.PrivateKeySize-ed25519.PublicKeySize : ed25519.PrivateKeySize]
		if subtle.ConstantTimeCompare(pk, redundantPk) == 0 {
			return nil, errors.New("expected redundant ed25519 public key to be redundant")
		}
		newKey := make([]byte, ed25519.PrivateKeySize)
		copy(newKey, data[:ed25519.PrivateKeySize])
		data = newKey
	case ed25519.PrivateKeySize:
	default:
		return nil, errors.Errorf(
			"expected ed25519 data size to be %d or %d, got %d",
			ed25519.PrivateKeySize,
			ed25519.PrivateKeySize+ed25519.PublicKeySize,
			len(data),
		)
	}
	return &Ed25519PrivateKey{k: ed25519.PrivateKey(data)}, nil
}
