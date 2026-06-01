package blockenc

import (
	"crypto/aes"
	"crypto/cipher"
)

// NewAES256GCM constructs a new AES-256-GCM block encryption method.
func NewAES256GCM(key []byte) (Method, error) {
	if len(key) < 32 {
		return nil, ErrShortKey
	}
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, err
	}
	c, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return newAeadCipher(c), nil
}
