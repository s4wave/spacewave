package envelope

import (
	"strconv"
	"strings"

	"github.com/zeebo/blake3"
)

var baseCryptoContext = "envelope 2026-02-08T00:00:00Z envelope crypto ctx v1."

// deriveEncKeyFromScalar derives a 32-byte encryption key from scalar bytes.
func deriveEncKeyFromScalar(scalarBytes []byte, envelopeID, context string) [32]byte {
	var key [32]byte
	ctx := buildKeyDerivationContext(envelopeID, context)
	blake3.DeriveKey(ctx, scalarBytes, key[:])
	return key
}

// buildKeyDerivationContext builds the BLAKE3 KDF context for key derivation.
// Uses length-prefixed fields to prevent context ambiguity.
func buildKeyDerivationContext(envelopeID, context string) string {
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("key_derivation ")
	b.WriteString(strconv.Itoa(len(envelopeID)))
	b.WriteByte(':')
	b.WriteString(envelopeID)
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(len(context)))
	b.WriteByte(':')
	b.WriteString(context)
	return b.String()
}

// buildGrantEncContext builds the encryption context for a grant ciphertext.
// Uses length-prefixed fields to prevent context ambiguity.
func buildGrantEncContext(envelopeID, context string, grantIndex int) string {
	var b strings.Builder
	b.WriteString(baseCryptoContext)
	b.WriteString("grant_enc ")
	b.WriteString(strconv.Itoa(len(envelopeID)))
	b.WriteByte(':')
	b.WriteString(envelopeID)
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(len(context)))
	b.WriteByte(':')
	b.WriteString(context)
	b.WriteByte(' ')
	b.WriteString(strconv.Itoa(grantIndex))
	return b.String()
}

// hashContext returns the BLAKE3 hash of a context string.
func hashContext(context string) []byte {
	h := blake3.Sum256([]byte(context))
	return h[:]
}
