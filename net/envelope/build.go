package envelope

import (
	"bytes"
	"encoding/hex"
	"io"

	"github.com/aperturerobotics/util/scrub"
	"github.com/cloudflare/circl/group"
	"github.com/cloudflare/circl/secretsharing"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/keypem"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/zeebo/blake3"
	"golang.org/x/crypto/chacha20poly1305"
)

// BuildEnvelope creates a new sealed Envelope from a plaintext payload.
//
// The payload is encrypted with XChaCha20-Poly1305 using a key derived from a
// Ristretto255 scalar. The scalar is split into Shamir shares and distributed
// across grants, each encrypted to the specified keypairs.
//
// Context must be the same string when calling UnlockEnvelope.
func BuildEnvelope(
	rnd io.Reader,
	context string,
	payload []byte,
	keypairs []crypto.PubKey,
	config *EnvelopeConfig,
) (*Envelope, error) {
	if len(payload) == 0 {
		return nil, ErrEmptyPayload
	}
	if len(keypairs) == 0 {
		return nil, ErrNoKeypairs
	}
	if config == nil || len(config.GetGrantConfigs()) == 0 {
		return nil, ErrNoGrants
	}

	threshold := config.GetThreshold()
	grants := config.GetGrantConfigs()

	// Validate keypair indexes and compute total shares.
	var totalShares uint32
	for _, gc := range grants {
		sc := gc.GetShareCount()
		if sc == 0 {
			sc = 1
		}
		totalShares += sc
		for _, idx := range gc.GetKeypairIndexes() {
			if int(idx) >= len(keypairs) {
				return nil, ErrInvalidKeypairIndex
			}
		}
	}
	if config.GetTotalShares() > 0 {
		totalShares = config.GetTotalShares()
	}
	if threshold > 0 && totalShares < threshold+1 {
		return nil, ErrInvalidThreshold
	}

	// Generate random Ristretto255 scalar as the master secret.
	g := group.Ristretto255
	secret := g.RandomNonZeroScalar(rnd)
	secretBytes, err := secret.MarshalBinary()
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(secretBytes)

	// Derive envelope ID from secret + context if not provided.
	envelopeID := config.GetEnvelopeId()
	if envelopeID == "" {
		h := blake3.New()
		_, _ = h.Write(secretBytes)
		_, _ = h.Write([]byte(context))
		var digest [32]byte
		_, _ = h.Digest().Read(digest[:])
		envelopeID = hex.EncodeToString(digest[:16])
	}

	// Derive encryption key and encrypt payload with XChaCha20-Poly1305.
	encKey := deriveEncKeyFromScalar(secretBytes, envelopeID, context)
	defer scrub.Scrub(encKey[:])
	aead, err := chacha20poly1305.NewX(encKey[:])
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := io.ReadFull(rnd, nonce); err != nil {
		return nil, err
	}
	ciphertext := aead.Seal(nonce, nonce, payload, nil)

	// Split secret into Shamir shares via CIRCL.
	// threshold=0: degree-0 polynomial, any single share suffices.
	// threshold=N: N+1 shares needed to recover.
	ss := secretsharing.New(rnd, uint(threshold), secret)
	shares := ss.Share(uint(totalShares))

	// Distribute shares across grants and encrypt each to its keypairs.
	shareIdx := 0
	envGrants := make([]*EnvelopeGrant, len(grants))
	for gi, gc := range grants {
		sc := int(gc.GetShareCount())
		if sc == 0 {
			sc = 1
		}

		inner := &EnvelopeGrantInner{}
		for j := 0; j < sc && shareIdx < len(shares); j++ {
			sid, err := shares[shareIdx].ID.MarshalBinary()
			if err != nil {
				return nil, err
			}
			sval, err := shares[shareIdx].Value.MarshalBinary()
			if err != nil {
				return nil, err
			}
			inner.Shares = append(inner.Shares, &EnvelopeShare{Id: sid, Value: sval})
			shareIdx++
		}

		innerData, err := inner.MarshalVT()
		if err != nil {
			return nil, err
		}

		kpIndexes := gc.GetKeypairIndexes()
		ciphertexts := make([][]byte, len(kpIndexes))
		encCtx := buildGrantEncContext(envelopeID, context, gi)
		for ci, kpIdx := range kpIndexes {
			ct, err := peer.EncryptToPubKey(keypairs[kpIdx], encCtx, innerData)
			if err != nil {
				scrub.Scrub(innerData)
				return nil, err
			}
			ciphertexts[ci] = ct
		}
		scrub.Scrub(innerData)

		envGrants[gi] = &EnvelopeGrant{
			KeypairIndexes: kpIndexes,
			Ciphertexts:    ciphertexts,
		}
	}

	// Build keypair entries and assemble the envelope.
	envKeypairs := make([]*EnvelopeKeypair, len(keypairs))
	for i, kp := range keypairs {
		raw, err := keypem.MarshalPubKeyPem(kp)
		if err != nil {
			return nil, err
		}
		envKeypairs[i] = &EnvelopeKeypair{PubKey: raw}
	}

	return &Envelope{
		EnvelopeId:  envelopeID,
		ContextHash: hashContext(context),
		Threshold:   threshold,
		Ciphertext:  ciphertext,
		Grants:      envGrants,
		Keypairs:    envKeypairs,
	}, nil
}

// matchPrivKeys finds which envelope keypairs each private key corresponds to.
// Returns a map from envelope keypair index to the matching private key.
func matchPrivKeys(env *Envelope, privKeys []crypto.PrivKey) (map[int]crypto.PrivKey, error) {
	// Pre-compute PEM bytes for each private key's public key.
	type privEntry struct {
		pem []byte
		key crypto.PrivKey
	}
	privEntries := make([]privEntry, 0, len(privKeys))
	for _, priv := range privKeys {
		pem, err := keypem.MarshalPubKeyPem(priv.GetPublic())
		if err != nil {
			continue
		}
		privEntries = append(privEntries, privEntry{pem: pem, key: priv})
	}

	result := make(map[int]crypto.PrivKey)
	for ki, ekp := range env.GetKeypairs() {
		pubBytes := ekp.GetPubKey()
		for _, pe := range privEntries {
			if bytes.Equal(pubBytes, pe.pem) {
				result[ki] = pe.key
				break
			}
		}
	}
	for i := range privEntries {
		scrub.Scrub(privEntries[i].pem)
	}
	return result, nil
}
