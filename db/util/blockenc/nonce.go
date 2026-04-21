package blockenc

import (
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
