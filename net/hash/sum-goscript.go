//go:build goscript

package hash

import (
	"crypto/sha256"

	"github.com/pkg/errors"
	"github.com/zeebo/blake3"
)

func sumHashType(h HashType, data []byte) ([]byte, error) {
	switch h {
	case HashType_HashType_SHA256:
		h := sha256.Sum256(data)
		return h[:], nil
	case HashType_HashType_BLAKE3:
		h := blake3.Sum256(data)
		return h[:], nil
	default:
		return nil, errors.Errorf("hash type unsupported in goscript: %v", h.String())
	}
}

func subtleCryptoDigest(name string, data []byte) ([]byte, error) {
	if name != "SHA-256" {
		return nil, errors.Errorf("hash digest unsupported in goscript: %s", name)
	}
	h := sha256.Sum256(data)
	return h[:], nil
}
