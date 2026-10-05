package provider_local

import (
	"crypto/rand"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
)

// TestUnlinkDeviceRefusesUnknownPeer checks that unlinking a peer that is
// neither a Session nor a paired Device of the account fails instead of
// reporting a revocation that did nothing.
func TestUnlinkDeviceRefusesUnknownPeer(t *testing.T) {
	// Start a local provider account.
	ctx := t.Context()
	_, _, acc, _, release := setupProviderAndSessionInternal(ctx, t)
	defer release()

	// Unlink a fresh peer the account has never seen.
	_, pub, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := peer.IDFromPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	err = acc.UnlinkDevice(ctx, unknown)
	if err == nil || !strings.Contains(err.Error(), "not a session or paired device") {
		t.Fatalf("UnlinkDevice(unknown) = %v, want a refusal", err)
	}
}
