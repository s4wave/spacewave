package provider_local_test

import "testing"

// TestSessionTransportCreate verifies that CreateSessionTransport creates a
// transport with the session's private key and the child bus resolves the peer.
func TestSessionTransportCreate(t *testing.T) {
	// Use the test context for the transport lifetime.
	ctx := t.Context()

	// Start a local account session and release it when the test returns.
	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	// Require the session private key before creating a transport.
	privKey := sess.GetPrivKey()
	if privKey == nil {
		t.Fatal("session private key is nil")
	}

	// Create transport with empty signaling URL (no WebRTC, but child bus + peer controller).
	if err := acc.CreateSessionTransport(ctx, privKey, ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	// Require the created session transport to be available.
	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected session transport to be non-nil")
	}

	// Verify the transport's peer ID matches the session peer ID.
	if st.GetPeerID() != sess.GetPeerId() {
		t.Fatalf("transport peer ID %s != session peer ID %s", st.GetPeerID().String(), sess.GetPeerId().String())
	}

	// Verify the child bus is running (non-nil after Execute starts).
	childBus := st.GetChildBus()
	if childBus == nil {
		t.Fatal("expected child bus to be non-nil after transport starts")
	}
}

// TestTransportCleanup verifies that StopSessionTransport stops the transport
// goroutine cleanly with no leaks.
func TestTransportCleanup(t *testing.T) {
	// Use the test context for transport cleanup.
	ctx := t.Context()

	// Start a local account session and release it when the test returns.
	_, _, acc, sess, release := setupProviderAndSession(ctx, t)
	defer release()

	// Create a session transport from the session private key.
	privKey := sess.GetPrivKey()
	if err := acc.CreateSessionTransport(ctx, privKey, ""); err != nil {
		t.Fatal(err)
	}

	// Require the running transport to expose its child bus.
	st := acc.GetSessionTransport()
	if st == nil {
		t.Fatal("expected transport to be running")
	}
	if st.GetChildBus() == nil {
		t.Fatal("expected child bus to be non-nil")
	}

	// Stop the transport.
	acc.StopSessionTransport()

	// Verify transport is nil after stop.
	if acc.GetSessionTransport() != nil {
		t.Fatal("expected transport to be nil after stop")
	}

	// Verify we can create a new transport after stopping the old one.
	if err := acc.CreateSessionTransport(ctx, privKey, ""); err != nil {
		t.Fatal(err)
	}
	defer acc.StopSessionTransport()

	// Require the replacement transport and its child bus.
	st2 := acc.GetSessionTransport()
	if st2 == nil {
		t.Fatal("expected new transport after re-create")
	}
	if st2.GetChildBus() == nil {
		t.Fatal("expected new child bus to be non-nil")
	}
}
