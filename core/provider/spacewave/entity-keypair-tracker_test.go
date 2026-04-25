package provider_spacewave

import (
	"crypto/ed25519"
	"crypto/rand"
	"testing"

	bifrost_crypto "github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestEntityKeypairTrackerLockScrubsKey verifies single-key lock scrubs bytes.
func TestEntityKeypairTrackerLockScrubsKey(t *testing.T) {
	tracker := NewEntityKeypairTracker()
	priv, pid, std := generateEntityKey(t)

	tracker.Unlock(pid, priv)
	assertHasNonZeroKeyBytes(t, std)

	tracker.Lock(pid)

	if tracker.IsUnlocked(pid) {
		t.Fatal("expected key to be locked")
	}
	assertZeroKeyBytes(t, std)
}

// TestEntityKeypairTrackerLockAllScrubsKeys verifies lock-all scrubs all keys.
func TestEntityKeypairTrackerLockAllScrubsKeys(t *testing.T) {
	tracker := NewEntityKeypairTracker()
	priv1, pid1, std1 := generateEntityKey(t)
	priv2, pid2, std2 := generateEntityKey(t)

	tracker.Unlock(pid1, priv1)
	tracker.Unlock(pid2, priv2)
	assertHasNonZeroKeyBytes(t, std1)
	assertHasNonZeroKeyBytes(t, std2)

	tracker.LockAll()

	if tracker.GetUnlockedCount() != 0 {
		t.Fatalf("expected no unlocked keys, got %d", tracker.GetUnlockedCount())
	}
	assertZeroKeyBytes(t, std1)
	assertZeroKeyBytes(t, std2)
}

// TestEntityKeypairTrackerUnlockReplacementScrubsOldKey verifies replacement
// scrubs the previously cached key bytes.
func TestEntityKeypairTrackerUnlockReplacementScrubsOldKey(t *testing.T) {
	tracker := NewEntityKeypairTracker()
	priv1, pid, std1 := generateEntityKey(t)
	priv2, _, std2 := generateEntityKey(t)

	tracker.Unlock(pid, priv1)
	assertHasNonZeroKeyBytes(t, std1)

	tracker.Unlock(pid, priv2)

	assertZeroKeyBytes(t, std1)
	assertHasNonZeroKeyBytes(t, std2)
}

func generateEntityKey(t *testing.T) (bifrost_crypto.PrivKey, peer.ID, ed25519.PrivateKey) {
	t.Helper()
	priv, _, err := bifrost_crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("generating key: %v", err)
	}
	pid, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("deriving peer ID: %v", err)
	}
	std := priv.(interface{ GetStdKey() ed25519.PrivateKey }).GetStdKey()
	return priv, pid, std
}

func assertHasNonZeroKeyBytes(t *testing.T, key ed25519.PrivateKey) {
	t.Helper()
	for _, b := range key {
		if b != 0 {
			return
		}
	}
	t.Fatal("expected key bytes to contain non-zero data")
}

func assertZeroKeyBytes(t *testing.T, key ed25519.PrivateKey) {
	t.Helper()
	for i, b := range key {
		if b != 0 {
			t.Fatalf("expected zeroed key bytes at index %d, got %d", i, b)
		}
	}
}
