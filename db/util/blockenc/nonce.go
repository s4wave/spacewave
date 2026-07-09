package blockenc

import (
	"crypto/sha256"

	"github.com/zeebo/blake3"
)

// nonceBlake3Context is the blake3 nonce constant.
// don't change this
const nonceBlake3Context = "aperturerobotics/hydra 2022-01-01 blockenc nonce v1."

// DeriveNonceBlake3 derives a nonce using blake3 key derivation.
// Fills "out" with data using all of src.
func DeriveNonceBlake3(src, out []byte) {
	blake3.DeriveKey(nonceBlake3Context, src, out)
}

// nonceSHA256Context is the SHA256 nonce constant.
const nonceSHA256Context = "aperturerobotics/hydra 2026-06-08 blockenc nonce sha256 v1."

// DeriveNonceSHA256 derives a nonce using SHA256.
// Fills "out" with data using all of src.
func DeriveNonceSHA256(src, out []byte) {
	deriveSHA256(nonceSHA256Context, src, out)
}

// DeriveKeySHA256 derives bytes from a context and source using SHA256.
func DeriveKeySHA256(context string, src, out []byte) {
	deriveSHA256(context, src, out)
}

func deriveSHA256(context string, src, out []byte) {
	for offset, counter := 0, uint64(0); offset < len(out); counter++ {
		h := sha256.New()
		h.Write([]byte(context))
		h.Write([]byte{0})
		h.Write(src)
		h.Write([]byte{
			byte(counter >> 56),
			byte(counter >> 48),
			byte(counter >> 40),
			byte(counter >> 32),
			byte(counter >> 24),
			byte(counter >> 16),
			byte(counter >> 8),
			byte(counter),
		})
		sum := h.Sum(nil)
		offset += copy(out[offset:], sum)
	}
}
