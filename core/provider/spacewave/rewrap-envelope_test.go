package provider_spacewave

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/aperturerobotics/util/scrub"
	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/envelope"
	"github.com/s4wave/spacewave/net/keypem"
)

// TestEnvelopeRewrap verifies that a session envelope built with entity
// keypair public keys can be unlocked by any single entity private key,
// and that re-wrapping with a new set of keys produces an envelope that
// includes the new key and excludes the removed key.
func TestEnvelopeRewrap(t *testing.T) {
	// Generate a session private key (the payload to protect).
	sessPriv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sessPEM, err := keypem.MarshalPrivKeyPem(sessPriv)
	if err != nil {
		t.Fatal(err)
	}
	defer scrub.Scrub(sessPEM)

	// Generate entity keypairs.
	entPriv1, entPub1 := genEntityKey(t)
	entPriv2, entPub2 := genEntityKey(t)

	// Build initial envelope with both entity keys.
	env1 := buildTestEnvelope(t, sessPEM, []bifrost_crypto.PubKey{entPub1, entPub2})

	// Either entity key alone should unlock.
	assertUnlock(t, env1, entPriv1, sessPEM, "entity key 1")
	assertUnlock(t, env1, entPriv2, sessPEM, "entity key 2")

	// Add a third entity key and re-wrap.
	entPriv3, entPub3 := genEntityKey(t)
	env2 := buildTestEnvelope(t, sessPEM, []bifrost_crypto.PubKey{entPub1, entPub2, entPub3})

	// All three should unlock.
	assertUnlock(t, env2, entPriv1, sessPEM, "entity key 1 after add")
	assertUnlock(t, env2, entPriv2, sessPEM, "entity key 2 after add")
	assertUnlock(t, env2, entPriv3, sessPEM, "entity key 3 after add")

	// Remove entity key 1 and re-wrap with only keys 2 and 3.
	env3 := buildTestEnvelope(t, sessPEM, []bifrost_crypto.PubKey{entPub2, entPub3})

	// Key 1 should no longer unlock.
	assertNoUnlock(t, env3, entPriv1, "removed entity key 1")

	// Keys 2 and 3 should still unlock.
	assertUnlock(t, env3, entPriv2, sessPEM, "entity key 2 after remove")
	assertUnlock(t, env3, entPriv3, sessPEM, "entity key 3 after remove")
}

// genEntityKey generates an Ed25519 entity keypair for testing.
func genEntityKey(t *testing.T) (bifrost_crypto.PrivKey, bifrost_crypto.PubKey) {
	t.Helper()
	priv, pub, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

// buildTestEnvelope builds a Shamir envelope with threshold=0 (1-of-N).
func buildTestEnvelope(t *testing.T, payload []byte, pubKeys []bifrost_crypto.PubKey) *envelope.Envelope {
	t.Helper()
	grantConfigs := make([]*envelope.EnvelopeGrantConfig, len(pubKeys))
	for i := range pubKeys {
		grantConfigs[i] = &envelope.EnvelopeGrantConfig{
			ShareCount:     1,
			KeypairIndexes: []uint32{uint32(i)}, //nolint:gosec
		}
	}
	config := &envelope.EnvelopeConfig{
		Threshold:    0,
		GrantConfigs: grantConfigs,
	}
	env, err := envelope.BuildEnvelope(rand.Reader, envelopeContext, payload, pubKeys, config)
	if err != nil {
		t.Fatal(err)
	}
	return env
}

// assertUnlock verifies the envelope can be unlocked with the given
// private key and the recovered payload matches expected.
func assertUnlock(t *testing.T, env *envelope.Envelope, priv bifrost_crypto.PrivKey, expected []byte, label string) {
	t.Helper()
	got, result, err := envelope.UnlockEnvelope(envelopeContext, env, []bifrost_crypto.PrivKey{priv})
	if err != nil {
		t.Fatalf("[%s] unlock error: %v", label, err)
	}
	if !result.GetSuccess() {
		t.Fatalf("[%s] expected success, got shares_available=%d shares_needed=%d",
			label, result.GetSharesAvailable(), result.GetSharesNeeded())
	}
	if !bytes.Equal(got, expected) {
		t.Fatalf("[%s] payload mismatch", label)
	}
}

// assertNoUnlock verifies the envelope cannot be unlocked with the given private key.
func assertNoUnlock(t *testing.T, env *envelope.Envelope, priv bifrost_crypto.PrivKey, label string) {
	t.Helper()
	got, result, err := envelope.UnlockEnvelope(envelopeContext, env, []bifrost_crypto.PrivKey{priv})
	if err != nil {
		t.Fatalf("[%s] unexpected error: %v", label, err)
	}
	if got != nil || result.GetSuccess() {
		t.Fatalf("[%s] expected failure but envelope was unlocked", label)
	}
}
