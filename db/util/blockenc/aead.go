package blockenc

import "crypto/cipher"

// NonceDeriver derives a deterministic nonce for an encrypted block body.
type NonceDeriver func(src, out []byte)

// aeadCipher uses a cipher AEAD object to encrypt/decrypt.
type aeadCipher struct {
	c           cipher.AEAD
	deriveNonce NonceDeriver
}

// newAeadCipher constructs a new aead cipher.
func newAeadCipher(c cipher.AEAD, deriveNonce NonceDeriver) *aeadCipher {
	return &aeadCipher{c: c, deriveNonce: deriveNonce}
}

// Encrypt encrypts the block and returns the encrypted buf.
func (b *aeadCipher) Encrypt(alloc AllocFn, src []byte) ([]byte, error) {
	nonceSize := b.c.NonceSize()
	outSize := nonceSize + len(src) + b.c.Overhead()
	nonce := alloc(outSize)[:nonceSize]
	b.deriveNonce(src, nonce)
	// note: Seal appends the data to nonce
	encrypted := b.c.Seal(nonce, nonce, src, nil)
	return encrypted, nil
}

// Decrypt decrypts the whole block and returns the decrypted buf.
func (b *aeadCipher) Decrypt(alloc AllocFn, src []byte) ([]byte, error) {
	nonceSize := b.c.NonceSize()
	if len(src) < nonceSize+1 {
		return nil, ErrShortMsg
	}
	nonce, ciphertext := src[:nonceSize], src[nonceSize:]
	dst := alloc(len(ciphertext))[:0]
	return b.c.Open(dst, nonce, ciphertext, nil)
}

// _ is a type assertion
var _ Method = (*aeadCipher)(nil)
