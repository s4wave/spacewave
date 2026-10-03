package blockenc

import (
	"crypto/aes"
	"crypto/cipher"
)

// NewAES256GCM constructs a new AES-256-GCM block encryption method.
func NewAES256GCM(key []byte) (Method, error) {
	// Require enough key bytes for AES-256 block encryption.
	if len(key) < 32 {
		return nil, ErrShortKey
	}

	// Construct the AES block cipher from the first 256 key bits.
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, err
	}

	// Wrap the AES cipher with GCM authentication and deterministic block nonces.
	c, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return newAeadCipher(c, DeriveNonceSHA256), nil
}
