//go:build !js && !goscript

package hash

import (
	"crypto/sha1" //nolint:gosec // Git object storage requires SHA-1.
	"crypto/sha256"

	"github.com/s4wave/spacewave/net/crypto/blake3"
)

func sumHashType(h HashType, data []byte) ([]byte, error) {
	switch h {
	case HashType_HashType_SHA256:
		h := sha256.Sum256(data)
		return h[:], nil
	case HashType_HashType_SHA1:
		h := sha1.Sum(data) //nolint:gosec
		return h[:], nil
	case HashType_HashType_BLAKE3:
		h := blake3.Sum256(data)
		return h[:], nil
	default:
		return nil, newUnsupportedHashTypeError(h, "hash type unknown: "+h.String())
	}
}
