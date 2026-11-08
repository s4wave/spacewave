// Package sha256 computes SHA-256 like crypto/sha256, with AVX code for x86-64
// processors that have AVX but not AVX2, where crypto/sha256 runs its generic
// code. On such a processor (a Xeon E5-2680 v2) the AVX code hashes 4 KB at
// 260 MB/s where crypto/sha256 reaches 145 MB/s, and 64 bytes in 551 ns where
// it takes 967 ns. Every other processor and build uses crypto/sha256.
package sha256

import "crypto/sha256"

// Size is the size of a SHA-256 checksum in bytes.
const Size = sha256.Size

// BlockSize is the block size of SHA-256 in bytes.
const BlockSize = sha256.BlockSize
