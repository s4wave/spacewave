package blockenc

import "golang.org/x/crypto/chacha20poly1305"

// NewXChaCha20Poly1305 constructs a new cipher.
func NewXChaCha20Poly1305(key []byte) (Method, error) {
	if len(key) < chacha20poly1305.KeySize {
		return nil, ErrShortKey
	}
	c, err := chacha20poly1305.NewX(key[:chacha20poly1305.KeySize])
	if err != nil {
		return nil, err
	}
	return newAeadCipher(c), nil
}
