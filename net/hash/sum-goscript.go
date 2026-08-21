//go:build goscript

package hash

import (
	"crypto/sha1" //nolint:gosec // Git object storage requires SHA-1.
	"crypto/sha256"

	"github.com/pkg/errors"
)

// sumHashType digests data with the GoScript-supported hash for h.
func sumHashType(h HashType, data []byte) ([]byte, error) {
	switch h {
	case HashType_HashType_SHA256:
		h := sha256.Sum256(data)
		return h[:], nil
	case HashType_HashType_SHA1:
		h := sha1.Sum(data)
		return h[:], nil
	default:
		return nil, newUnsupportedHashTypeError(h, "hash type unsupported in goscript: "+h.String())
	}
}

// subtleCryptoDigest digests data with the GoScript SubtleCrypto algorithm
// named by name.
func subtleCryptoDigest(name string, data []byte) ([]byte, error) {
	if name != "SHA-256" && name != "SHA-1" {
		return nil, errors.Errorf("hash digest unsupported in goscript: %s", name)
	}
	if name == "SHA-1" {
		h := sha1.Sum(data)
		return h[:], nil
	}
	h := sha256.Sum256(data)
	return h[:], nil
}
