// Package session_lock provides shared lock/unlock operations for session
// private keys. Both local and spacewave providers use this package.
package session_lock

import (
	"github.com/aperturerobotics/util/scrub"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/zeebo/blake3"
)

// DeriveStorageKey derives the auto-unlock storage key from the volume's
// persistent peer private key via blake3 key derivation.
func DeriveStorageKey(volPeerPrivKey crypto.PrivKey) ([32]byte, error) {
	raw, err := volPeerPrivKey.Raw()
	if err != nil {
		return [32]byte{}, err
	}
	defer scrub.Scrub(raw)
	var key [32]byte
	blake3.DeriveKey("session-privkey-v2", raw, key[:])
	return key, nil
}

// EncryptAutoUnlock encrypts session privkey PEM with the storage key.
func EncryptAutoUnlock(storageKey [32]byte, privPEM []byte) ([]byte, error) {
	method, err := blockenc.NewXChaCha20Poly1305(storageKey[:])
	if err != nil {
		return nil, err
	}
	return method.Encrypt(blockenc.DefaultAllocFn(), privPEM)
}

// DecryptAutoUnlock decrypts session privkey PEM with the storage key.
func DecryptAutoUnlock(storageKey [32]byte, encrypted []byte) ([]byte, error) {
	method, err := blockenc.NewXChaCha20Poly1305(storageKey[:])
	if err != nil {
		return nil, err
	}
	return method.Decrypt(blockenc.DefaultAllocFn(), encrypted)
}
