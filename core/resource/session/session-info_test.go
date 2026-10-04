package resource_session_test

import (
	"context"
	"testing"

	provider_local "github.com/s4wave/spacewave/core/provider/local"
	"github.com/s4wave/spacewave/core/sobject"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestGetSessionInfoReportsSigningPeer verifies a local account's session
// reports the peer that signs its Space operations, which is not the Session
// peer.
func TestGetSessionInfoReportsSigningPeer(t *testing.T) {
	// Start a test environment with a local account and one Space.
	ctx := context.Background()
	env := setupTestEnv(ctx, t)
	sessRef, _ := env.createSession(ctx, t)
	account := env.accessAccount(ctx, t, sessRef)
	env.createSpaceOnAccount(ctx, t, account, "SigningSpace")

	// Mount the Space's SharedObject to read the peer that signs its operations.
	spaceID := "signingspace-id"
	soRef := sobject.NewSharedObjectRef(
		"local",
		sessRef.GetProviderResourceRef().GetProviderAccountId(),
		spaceID,
		provider_local.SobjectBlockStoreID(spaceID),
	)
	so, releaseSO, err := account.MountSharedObject(ctx, soRef, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseSO()

	// Read the Session info through the Session resource.
	resource := env.buildSessionResource(ctx, t, sessRef)
	info, err := resource.GetSessionInfo(ctx, &s4wave_session.GetSessionInfoRequest{})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the signing peer is the SharedObject's peer, not the Session peer.
	if info.GetSigningPeerId() != so.GetPeerID().String() {
		t.Fatalf("signing peer = %q, want the SharedObject peer %q", info.GetSigningPeerId(), so.GetPeerID())
	}
	if info.GetSigningPeerId() == info.GetPeerId() {
		t.Fatalf("signing peer %q equals the Session peer", info.GetSigningPeerId())
	}
}
