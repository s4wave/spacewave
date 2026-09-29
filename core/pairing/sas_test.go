package pairing

import (
	"crypto/rand"
	"slices"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// generateTestKeypair generates an Ed25519 keypair and peer ID for testing.
func generateTestKeypair(t *testing.T) (crypto.PrivKey, crypto.PubKey, peer.ID) {
	// Generate the Ed25519 keypair and derive its peer ID.
	t.Helper()
	priv, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return priv, pub, pid
}

// TestDeriveSASEmoji_Deterministic verifies that the same inputs produce the same output.
func TestDeriveSASEmoji_Deterministic(t *testing.T) {
	// Generate both keypairs.
	privA, _, pidA := generateTestKeypair(t)
	_, pubB, pidB := generateTestKeypair(t)

	// Derive the SAS emoji twice from the same inputs.
	emoji1, err := DeriveSASEmoji(privA, pubB, pidA, pidB)
	if err != nil {
		t.Fatal(err)
	}
	emoji2, err := DeriveSASEmoji(privA, pubB, pidA, pidB)
	if err != nil {
		t.Fatal(err)
	}

	// Require both derivations to agree.
	if !slices.Equal(emoji1, emoji2) {
		t.Fatalf("expected deterministic output, got %v and %v", emoji1, emoji2)
	}
}

// TestDeriveSASEmoji_Symmetric verifies that both sides derive the same emoji sequence.
func TestDeriveSASEmoji_Symmetric(t *testing.T) {
	// Generate both keypairs.
	privA, pubA, pidA := generateTestKeypair(t)
	privB, pubB, pidB := generateTestKeypair(t)

	// Derive the SAS emoji from each side's perspective.
	emojiA, err := DeriveSASEmoji(privA, pubB, pidA, pidB)
	if err != nil {
		t.Fatal(err)
	}
	emojiB, err := DeriveSASEmoji(privB, pubA, pidB, pidA)
	if err != nil {
		t.Fatal(err)
	}

	// Require both sides to derive the same emoji.
	if !slices.Equal(emojiA, emojiB) {
		t.Fatalf("SAS mismatch: side A got %v, side B got %v", emojiA, emojiB)
	}
}

// TestDeriveSASEmoji_Length verifies the result is always exactly 6 emoji.
func TestDeriveSASEmoji_Length(t *testing.T) {
	// Generate both keypairs and derive the SAS emoji.
	privA, _, pidA := generateTestKeypair(t)
	_, pubB, pidB := generateTestKeypair(t)

	// Derive the SAS emoji from the same inputs.
	emoji, err := DeriveSASEmoji(privA, pubB, pidA, pidB)
	if err != nil {
		t.Fatal(err)
	}

	// Require exactly six emoji.
	if len(emoji) != 6 {
		t.Fatalf("expected 6 emoji, got %d", len(emoji))
	}
}

// TestDeriveSASEmoji_DifferentKeysProduceDifferentEmoji verifies that
// different keypairs produce different SAS sequences.
func TestDeriveSASEmoji_DifferentKeysProduceDifferentEmoji(t *testing.T) {
	// Generate three keypairs.
	privA, _, pidA := generateTestKeypair(t)
	_, pubB, pidB := generateTestKeypair(t)
	_, pubC, pidC := generateTestKeypair(t)

	// Derive the SAS emoji against two different remote keys.
	emojiAB, err := DeriveSASEmoji(privA, pubB, pidA, pidB)
	if err != nil {
		t.Fatal(err)
	}
	emojiAC, err := DeriveSASEmoji(privA, pubC, pidA, pidC)
	if err != nil {
		t.Fatal(err)
	}

	// Require the two sequences to differ.
	if slices.Equal(emojiAB, emojiAC) {
		t.Fatalf("different keys produced same SAS: %v", emojiAB)
	}
}

// TestDeriveSASEmoji_ValidEmoji verifies all returned strings are in the sasEmojiTable.
func TestDeriveSASEmoji_ValidEmoji(t *testing.T) {
	// Generate both keypairs and derive the SAS emoji.
	privA, _, pidA := generateTestKeypair(t)
	_, pubB, pidB := generateTestKeypair(t)

	// Derive the SAS emoji from the same inputs.
	emoji, err := DeriveSASEmoji(privA, pubB, pidA, pidB)
	if err != nil {
		t.Fatal(err)
	}

	// Require every derived emoji to appear in the table.
	emojiSet := make(map[string]struct{}, len(sasEmojiTable))
	for _, e := range sasEmojiTable {
		emojiSet[e] = struct{}{}
	}

	// Require each derived emoji to be a table entry.
	for i, e := range emoji {
		if _, ok := emojiSet[e]; !ok {
			t.Fatalf("emoji[%d] = %q is not in sasEmojiTable", i, e)
		}
	}
}
