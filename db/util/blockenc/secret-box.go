package blockenc

import (
	"golang.org/x/crypto/nacl/secretbox"
)

// secretBox uses nacl secretbox encryption.
type secretBox struct {
	key [32]byte
}

// NewSecretBox constructs a new nacl scret box method.
//
// The first 24 bytes of the encrypted block are the nonce.
// Nonce is derived from the blake3 keyderiv of the source.
func NewSecretBox(key []byte) (Method, error) {
	if len(key) < 32 {
		return nil, ErrShortKey
	}

	c := &secretBox{}
	copy(c.key[:], key)
	return c, nil
}

// Encrypt encrypts the block and returns the encrypted buf.
func (c *secretBox) Encrypt(alloc AllocFn, src []byte) ([]byte, error) {
	var nonce [24]byte
	DeriveNonceBlake3(src, nonce[:])

	outSize := len(nonce) + len(src) + secretbox.Overhead
	out := alloc(outSize)[:len(nonce)]
	copy(out, nonce[:])
	encrypted := secretbox.Seal(out, src, &nonce, &c.key)
	return encrypted, nil
}

// Decrypt decrypts the whole block and returns the decrypted buf.
func (c *secretBox) Decrypt(alloc AllocFn, src []byte) ([]byte, error) {
	if len(src) < 25 {
		return nil, ErrShortMsg
	}
	var nonce [24]byte
	copy(nonce[:], src)
	ciphertext := src[len(nonce):]
	out := alloc(len(ciphertext))[:0]
	decrypted, ok := secretbox.Open(out, ciphertext, &nonce, &c.key)
	if !ok {
		return nil, ErrDecryptFail
	}
	return decrypted, nil
}

// _ is a type assertion
var _ Method = ((*secretBox)(nil))
