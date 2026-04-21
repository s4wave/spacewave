package envelope

import (
	"bytes"
	"encoding/hex"

	"github.com/aperturerobotics/util/scrub"
	"github.com/cloudflare/circl/group"
	"github.com/cloudflare/circl/secretsharing"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"golang.org/x/crypto/chacha20poly1305"
)

// UnlockEnvelope attempts to decrypt an Envelope using the provided private keys.
//
// Returns (payload, result, nil) on success.
// Returns (nil, result, nil) when not enough shares are available (result has progress).
// Returns (nil, nil, err) on invalid envelope or other errors.
func UnlockEnvelope(
	context string,
	env *Envelope,
	privKeys []crypto.PrivKey,
) ([]byte, *EnvelopeUnlockResult, error) {
	if len(env.GetGrants()) == 0 {
		return nil, nil, ErrNoGrants
	}
	if len(env.GetKeypairs()) == 0 {
		return nil, nil, ErrNoKeypairs
	}

	// Verify context hash.
	expected := hashContext(context)
	if !bytes.Equal(env.GetContextHash(), expected) {
		return nil, nil, ErrContextMismatch
	}

	// Match private keys to envelope keypairs.
	matched, err := matchPrivKeys(env, privKeys)
	if err != nil {
		return nil, nil, err
	}

	threshold := env.GetThreshold()
	sharesNeeded := threshold + 1
	envelopeID := env.GetEnvelopeId()

	// Decrypt grants and collect shares, deduplicating by share ID.
	var collected []secretsharing.Share
	seen := make(map[string]struct{})
	var unlockedIndexes []uint32

	for gi, grant := range env.GetGrants() {
		kpIndexes := grant.GetKeypairIndexes()
		ciphertexts := grant.GetCiphertexts()
		if len(kpIndexes) != len(ciphertexts) {
			continue
		}

		// Try each matched keypair until one decrypts the grant.
		encCtx := buildGrantEncContext(envelopeID, context, gi)
		var innerData []byte
		for ci, kpIdx := range kpIndexes {
			priv, ok := matched[int(kpIdx)]
			if !ok {
				continue
			}
			dec, err := peer.DecryptWithPrivKey(priv, encCtx, ciphertexts[ci])
			if err != nil {
				continue
			}
			innerData = dec
			break
		}
		if innerData == nil {
			continue
		}

		inner := &EnvelopeGrantInner{}
		if err := inner.UnmarshalVT(innerData); err != nil {
			scrub.Scrub(innerData)
			continue
		}
		scrub.Scrub(innerData)
		unlockedIndexes = append(unlockedIndexes, uint32(gi)) //nolint:gosec // gi bounded by grants slice length

		// Extract shares from the grant, deduplicating by ID.
		g := group.Ristretto255
		for _, s := range inner.GetShares() {
			idKey := hex.EncodeToString(s.GetId())
			if _, dup := seen[idKey]; dup {
				continue
			}

			id := g.NewScalar()
			if err := id.UnmarshalBinary(s.GetId()); err != nil {
				continue
			}
			val := g.NewScalar()
			if err := val.UnmarshalBinary(s.GetValue()); err != nil {
				continue
			}

			seen[idKey] = struct{}{}
			collected = append(collected, secretsharing.Share{ID: id, Value: val})
		}
	}

	result := &EnvelopeUnlockResult{
		SharesAvailable:      uint32(len(collected)), //nolint:gosec // bounded by grant share count
		SharesNeeded:         sharesNeeded,
		UnlockedGrantIndexes: unlockedIndexes,
	}
	if uint32(len(collected)) < sharesNeeded { //nolint:gosec // bounded by grant share count
		return nil, result, nil
	}

	// Recover the secret scalar via CIRCL Lagrange interpolation.
	recovered, err := secretsharing.Recover(uint(threshold), collected)
	if err != nil {
		return nil, nil, err
	}
	scalarBytes, err := recovered.MarshalBinary()
	if err != nil {
		return nil, nil, err
	}
	defer scrub.Scrub(scalarBytes)

	// Derive encryption key and decrypt payload.
	encKey := deriveEncKeyFromScalar(scalarBytes, envelopeID, context)
	defer scrub.Scrub(encKey[:])
	aead, err := chacha20poly1305.NewX(encKey[:])
	if err != nil {
		return nil, nil, err
	}

	ct := env.GetCiphertext()
	if len(ct) < aead.NonceSize() {
		return nil, nil, ErrDecryptionFailed
	}
	nonce := ct[:aead.NonceSize()]
	payload, err := aead.Open(nil, nonce, ct[aead.NonceSize():], nil)
	if err != nil {
		return nil, nil, ErrDecryptionFailed
	}

	result.Success = true
	return payload, result, nil
}
