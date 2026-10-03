package envelope

import (
	"bytes"
	"crypto/rand"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
)

// genKey generates a new Ed25519 keypair for testing.
func genKey(t *testing.T) (crypto.PrivKey, crypto.PubKey) {
	t.Helper()
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub
}

func TestBuildAndUnlockEnvelope(t *testing.T) {
	// Prepare the envelope payload and shared encryption context.
	payload := []byte("test secret payload data")
	ctx := "test context v1"

	// Verify a single grant unlocks an envelope with threshold zero.
	t.Run("Single grant threshold zero", func(t *testing.T) {
		// Prepare the recipient keypair for the single envelope grant.
		priv, pub := genKey(t)

		// Seal the payload in an envelope with one recipient grant.
		env, err := BuildEnvelope(rand.Reader, ctx, payload, []crypto.PubKey{pub}, &EnvelopeConfig{
			Threshold: 0,
			GrantConfigs: []*EnvelopeGrantConfig{{
				ShareCount:     1,
				KeypairIndexes: []uint32{0},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Unlock the envelope with its recipient private key.
		got, result, err := UnlockEnvelope(ctx, env, []crypto.PrivKey{priv})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the envelope unlock succeeds and recovers the original payload.
		if !result.GetSuccess() {
			t.Fatal("expected success")
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("payload mismatch: got %q, want %q", got, payload)
		}
	})

	// Verify an unrelated private key cannot unlock the envelope.
	t.Run("Single grant wrong key", func(t *testing.T) {
		// Prepare distinct recipient and unrelated keypairs.
		_, pub := genKey(t)
		wrongPriv, _ := genKey(t)

		// Seal the payload for the intended envelope recipient.
		env, err := BuildEnvelope(rand.Reader, ctx, payload, []crypto.PubKey{pub}, &EnvelopeConfig{
			Threshold: 0,
			GrantConfigs: []*EnvelopeGrantConfig{{
				ShareCount:     1,
				KeypairIndexes: []uint32{0},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Attempt to unlock the envelope with the unrelated private key.
		got, result, err := UnlockEnvelope(ctx, env, []crypto.PrivKey{wrongPriv})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the envelope yields no payload or recovered shares.
		if got != nil {
			t.Fatal("expected nil payload with wrong key")
		}
		if result.GetSuccess() {
			t.Fatal("expected failure")
		}
		if result.GetSharesAvailable() != 0 {
			t.Fatalf("expected 0 shares available, got %d", result.GetSharesAvailable())
		}
	})

	// Verify either grant unlocks an envelope with threshold zero.
	t.Run("Multiple grants threshold zero", func(t *testing.T) {
		// Prepare independent keypairs for the two envelope grants.
		priv1, pub1 := genKey(t)
		priv2, pub2 := genKey(t)

		// Seal the payload with one envelope grant for each recipient.
		env, err := BuildEnvelope(rand.Reader, ctx, payload, []crypto.PubKey{pub1, pub2}, &EnvelopeConfig{
			Threshold: 0,
			GrantConfigs: []*EnvelopeGrantConfig{
				{ShareCount: 1, KeypairIndexes: []uint32{0}},
				{ShareCount: 1, KeypairIndexes: []uint32{1}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Either key alone should unlock.
		got, result, err := UnlockEnvelope(ctx, env, []crypto.PrivKey{priv1})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the first grant recovers the complete envelope payload.
		if !result.GetSuccess() {
			t.Fatal("expected success with key 1")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch with key 1")
		}

		// Unlock the envelope independently with the second recipient key.
		got, result, err = UnlockEnvelope(ctx, env, []crypto.PrivKey{priv2})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the second grant recovers the complete envelope payload.
		if !result.GetSuccess() {
			t.Fatal("expected success with key 2")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch with key 2")
		}
	})

	// Verify two recipient keys satisfy the envelope share threshold.
	t.Run("Multi-factor threshold", func(t *testing.T) {
		// Prepare independent keypairs for the required envelope grants.
		priv1, pub1 := genKey(t)
		priv2, pub2 := genKey(t)

		// Seal the payload in an envelope requiring two shares.
		env, err := BuildEnvelope(rand.Reader, ctx, payload, []crypto.PubKey{pub1, pub2}, &EnvelopeConfig{
			Threshold: 1, // need 2 shares
			GrantConfigs: []*EnvelopeGrantConfig{
				{ShareCount: 1, KeypairIndexes: []uint32{0}},
				{ShareCount: 1, KeypairIndexes: []uint32{1}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Both keys needed.
		got, result, err := UnlockEnvelope(ctx, env, []crypto.PrivKey{priv1, priv2})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the combined grants recover the complete envelope payload.
		if !result.GetSuccess() {
			t.Fatal("expected success with both keys")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch")
		}
	})

	// Verify a partial set of recipient keys reports envelope recovery progress.
	t.Run("Multi-factor partial", func(t *testing.T) {
		// Prepare two recipient keypairs while retaining only one private key.
		priv1, pub1 := genKey(t)
		_, pub2 := genKey(t)

		// Seal the payload in an envelope requiring both recipient shares.
		env, err := BuildEnvelope(rand.Reader, ctx, payload, []crypto.PubKey{pub1, pub2}, &EnvelopeConfig{
			Threshold: 1,
			GrantConfigs: []*EnvelopeGrantConfig{
				{ShareCount: 1, KeypairIndexes: []uint32{0}},
				{ShareCount: 1, KeypairIndexes: []uint32{1}},
			},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Only one key -- insufficient.
		got, result, err := UnlockEnvelope(ctx, env, []crypto.PrivKey{priv1})
		if err != nil {
			t.Fatal(err)
		}

		// Verify partial envelope recovery retains one share without revealing the payload.
		if got != nil {
			t.Fatal("expected nil payload")
		}
		if result.GetSuccess() {
			t.Fatal("expected failure")
		}
		if result.GetSharesAvailable() != 1 {
			t.Fatalf("expected 1 share available, got %d", result.GetSharesAvailable())
		}
		if result.GetSharesNeeded() != 2 {
			t.Fatalf("expected 2 shares needed, got %d", result.GetSharesNeeded())
		}
	})

	// Verify envelope encryption binds the payload to its caller context.
	t.Run("Context mismatch", func(t *testing.T) {
		// Prepare a recipient keypair for the context-bound envelope.
		priv, pub := genKey(t)

		// Seal the payload with the first envelope context.
		env, err := BuildEnvelope(rand.Reader, "context A", payload, []crypto.PubKey{pub}, &EnvelopeConfig{
			Threshold: 0,
			GrantConfigs: []*EnvelopeGrantConfig{{
				ShareCount:     1,
				KeypairIndexes: []uint32{0},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Attempt envelope recovery under a different caller context.
		_, _, err = UnlockEnvelope("context B", env, []crypto.PrivKey{priv})

		// Verify the envelope rejects the mismatched context.
		if err != ErrContextMismatch {
			t.Fatalf("expected ErrContextMismatch, got %v", err)
		}
	})

	// Verify envelope construction rejects an empty payload.
	t.Run("Empty payload", func(t *testing.T) {
		// Prepare an envelope recipient for empty payload validation.
		_, pub := genKey(t)

		// Attempt to seal an empty payload in a single-grant envelope.
		_, err := BuildEnvelope(rand.Reader, ctx, nil, []crypto.PubKey{pub}, &EnvelopeConfig{
			Threshold: 0,
			GrantConfigs: []*EnvelopeGrantConfig{{
				ShareCount:     1,
				KeypairIndexes: []uint32{0},
			}},
		})

		// Verify envelope construction reports the empty payload error.
		if err != ErrEmptyPayload {
			t.Fatalf("expected ErrEmptyPayload, got %v", err)
		}
	})

	// Verify either recipient key unlocks a shared envelope grant.
	t.Run("Grant to multiple keypairs", func(t *testing.T) {
		// Prepare independent keypairs for the shared envelope grant.
		priv1, pub1 := genKey(t)
		priv2, pub2 := genKey(t)

		// Seal the payload with one grant encrypted to both recipients.
		env, err := BuildEnvelope(rand.Reader, ctx, payload, []crypto.PubKey{pub1, pub2}, &EnvelopeConfig{
			Threshold: 0,
			GrantConfigs: []*EnvelopeGrantConfig{{
				ShareCount:     1,
				KeypairIndexes: []uint32{0, 1},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}

		// Either key should decrypt the grant.
		got, result, err := UnlockEnvelope(ctx, env, []crypto.PrivKey{priv1})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the first recipient recovers the shared grant payload.
		if !result.GetSuccess() {
			t.Fatal("expected success with key 1")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch with key 1")
		}

		// Unlock the shared envelope grant with the second recipient key.
		got, result, err = UnlockEnvelope(ctx, env, []crypto.PrivKey{priv2})
		if err != nil {
			t.Fatal(err)
		}

		// Verify the second recipient recovers the shared grant payload.
		if !result.GetSuccess() {
			t.Fatal("expected success with key 2")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch with key 2")
		}
	})
}
