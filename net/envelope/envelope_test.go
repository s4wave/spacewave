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
	payload := []byte("test secret payload data")
	ctx := "test context v1"

	t.Run("Single grant threshold zero", func(t *testing.T) {
		priv, pub := genKey(t)
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

		got, result, err := UnlockEnvelope(ctx, env, []crypto.PrivKey{priv})
		if err != nil {
			t.Fatal(err)
		}
		if !result.GetSuccess() {
			t.Fatal("expected success")
		}
		if !bytes.Equal(got, payload) {
			t.Fatalf("payload mismatch: got %q, want %q", got, payload)
		}
	})

	t.Run("Single grant wrong key", func(t *testing.T) {
		_, pub := genKey(t)
		wrongPriv, _ := genKey(t)

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

		got, result, err := UnlockEnvelope(ctx, env, []crypto.PrivKey{wrongPriv})
		if err != nil {
			t.Fatal(err)
		}
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

	t.Run("Multiple grants threshold zero", func(t *testing.T) {
		priv1, pub1 := genKey(t)
		priv2, pub2 := genKey(t)

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
		if !result.GetSuccess() {
			t.Fatal("expected success with key 1")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch with key 1")
		}

		got, result, err = UnlockEnvelope(ctx, env, []crypto.PrivKey{priv2})
		if err != nil {
			t.Fatal(err)
		}
		if !result.GetSuccess() {
			t.Fatal("expected success with key 2")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch with key 2")
		}
	})

	t.Run("Multi-factor threshold", func(t *testing.T) {
		priv1, pub1 := genKey(t)
		priv2, pub2 := genKey(t)

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
		if !result.GetSuccess() {
			t.Fatal("expected success with both keys")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch")
		}
	})

	t.Run("Multi-factor partial", func(t *testing.T) {
		priv1, pub1 := genKey(t)
		_, pub2 := genKey(t)

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

	t.Run("Context mismatch", func(t *testing.T) {
		priv, pub := genKey(t)
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

		_, _, err = UnlockEnvelope("context B", env, []crypto.PrivKey{priv})
		if err != ErrContextMismatch {
			t.Fatalf("expected ErrContextMismatch, got %v", err)
		}
	})

	t.Run("Empty payload", func(t *testing.T) {
		_, pub := genKey(t)
		_, err := BuildEnvelope(rand.Reader, ctx, nil, []crypto.PubKey{pub}, &EnvelopeConfig{
			Threshold: 0,
			GrantConfigs: []*EnvelopeGrantConfig{{
				ShareCount:     1,
				KeypairIndexes: []uint32{0},
			}},
		})
		if err != ErrEmptyPayload {
			t.Fatalf("expected ErrEmptyPayload, got %v", err)
		}
	})

	t.Run("Grant to multiple keypairs", func(t *testing.T) {
		priv1, pub1 := genKey(t)
		priv2, pub2 := genKey(t)

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
		if !result.GetSuccess() {
			t.Fatal("expected success with key 1")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch with key 1")
		}

		got, result, err = UnlockEnvelope(ctx, env, []crypto.PrivKey{priv2})
		if err != nil {
			t.Fatal(err)
		}
		if !result.GetSuccess() {
			t.Fatal("expected success with key 2")
		}
		if !bytes.Equal(got, payload) {
			t.Fatal("payload mismatch with key 2")
		}
	})
}
