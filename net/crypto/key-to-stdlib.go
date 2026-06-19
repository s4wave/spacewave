package crypto

import (
	stdcrypto "crypto"
	"crypto/ed25519"
)

// KeyPairFromStdKey wraps standard library private keys in bifrost crypto keys.
func KeyPairFromStdKey(priv stdcrypto.PrivateKey) (PrivKey, PubKey, error) {
	if priv == nil {
		return nil, nil, ErrNilPrivateKey
	}
	switch p := priv.(type) {
	case *ed25519.PrivateKey:
		pub, _ := p.Public().(ed25519.PublicKey)
		return &Ed25519PrivateKey{*p}, &Ed25519PublicKey{pub}, nil
	case ed25519.PrivateKey:
		pub, _ := p.Public().(ed25519.PublicKey)
		return &Ed25519PrivateKey{p}, &Ed25519PublicKey{pub}, nil
	default:
		return nil, nil, ErrBadKeyType
	}
}

// PrivKeyToStdKey converts a bifrost private key to a standard library private key.
func PrivKeyToStdKey(priv PrivKey) (stdcrypto.PrivateKey, error) {
	if priv == nil {
		return nil, ErrNilPrivateKey
	}
	switch p := priv.(type) {
	case *Ed25519PrivateKey:
		return &p.k, nil
	default:
		return nil, ErrBadKeyType
	}
}

// PubKeyToStdKey converts a bifrost public key to a standard library public key.
func PubKeyToStdKey(pub PubKey) (stdcrypto.PublicKey, error) {
	if pub == nil {
		return nil, ErrNilPublicKey
	}
	switch p := pub.(type) {
	case *Ed25519PublicKey:
		return p.k, nil
	default:
		return nil, ErrBadKeyType
	}
}
