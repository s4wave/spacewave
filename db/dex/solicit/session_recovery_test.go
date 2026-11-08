package dex_solicit

import (
	"bytes"
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// recoveryWait bounds the wait for a restored session in the recovery tests.
// The higher peer renews after renewGrace, so this leaves room for one retry.
const recoveryWait = renewGrace + renewRetryMin + time.Second

// TestSessionRecoversAfterRepeatedLoss ends the DEX session between two
// linked nodes several times while their link stays up, and checks that each
// time both nodes get a session back. The lower peer's renewal used to be an
// equivalent directive, which the bus merged into the previous renewal, so
// only the first loss was ever recovered.
func TestSessionRecoversAfterRepeatedLoss(t *testing.T) {
	// Link two nodes and wait for their sessions.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	a, b := newTestRecoveryPair(t, ctx)

	for round := 1; round <= 3; round++ {
		// End the session from one side; the stream close ends the other.
		prevA, prevB := a.dex.GetSessionStarts(), b.dex.GetSessionStarts()
		sess := getTestDexSession(a.dex, testRelayPeerID(t, ctx, b))
		if sess == nil {
			t.Fatalf("round %d: no session to end", round)
		}
		sess.close()

		// Both nodes must start a new session over the same link.
		waitTestRecovery(t, "new session on both nodes after loss "+strconv.Itoa(round), func() bool {
			return a.dex.GetSessionStarts() > prevA && b.dex.GetSessionStarts() > prevB &&
				len(openTestSessions(a.dex)) != 0 && len(openTestSessions(b.dex)) != 0
		})
	}
}

// TestHigherPeerRecoversOneSidedLoss ends only the higher peer's session with
// the lower peer while the link and the lower peer's session stay up, as when
// the lower peer never observes the end. The higher peer used to wait for the
// lower peer to renew, which it never did, so the higher peer had no session
// until the link remounted. A read that may not wait for peers, issued right
// after the loss, waits for the recovery instead of failing.
func TestHigherPeerRecoversOneSidedLoss(t *testing.T) {
	// Link two nodes and order them by peer ID.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	a, b := newTestRecoveryPair(t, ctx)
	lower, higher := a, b
	if testRelayPeerID(t, ctx, b) < testRelayPeerID(t, ctx, a) {
		lower, higher = b, a
	}
	lowerID := testRelayPeerID(t, ctx, lower)

	// Store a block only on the lower peer.
	data := []byte("block held only by the lower peer")
	ref := putTestRecoveryBlock(t, ctx, lower, data)

	// End the higher peer's session as its read loop would, leaving the
	// stream, and so the lower peer's session, open.
	sess := getTestDexSession(higher.dex, lowerID)
	if sess == nil {
		t.Fatal("higher peer has no session")
	}
	sess.setEndErr(errPeerClosedStream)
	sess.closed.Store(true)
	sess.onExit()
	if len(openTestSessions(higher.dex)) != 0 {
		t.Fatal("higher peer still lists a session")
	}
	if len(openTestSessions(lower.dex)) == 0 {
		t.Fatal("lower peer lost its session too")
	}

	// A read that does not wait for peers gets the block once the session
	// is restored.
	readCtx, _ := block.WithoutPeerWait(ctx)
	start := time.Now()
	stored, err := NewStore(higher.dex).GetStoredBlock(readCtx, ref)
	if err != nil {
		t.Fatalf("read after one-sided loss: %v", err)
	}
	if stored == nil || !bytes.Equal(stored.Data, data) {
		t.Fatalf("read after one-sided loss = %v", stored)
	}
	if elapsed := time.Since(start); elapsed > recoveryWait {
		t.Fatalf("read waited %s for the recovery", elapsed)
	}
	if len(openTestSessions(higher.dex)) == 0 {
		t.Fatal("higher peer has no session after the read")
	}
}

// TestHigherPeerRenewsAfterGrace checks the higher peer restores a one-sided
// loss on its own within renewGrace and a retry, without any read asking.
func TestHigherPeerRenewsAfterGrace(t *testing.T) {
	// Link two nodes and pick the higher peer.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(cancel)
	a, b := newTestRecoveryPair(t, ctx)
	lower, higher := a, b
	if testRelayPeerID(t, ctx, b) < testRelayPeerID(t, ctx, a) {
		lower, higher = b, a
	}

	// End only the higher peer's session.
	sess := getTestDexSession(higher.dex, testRelayPeerID(t, ctx, lower))
	if sess == nil {
		t.Fatal("higher peer has no session")
	}
	sess.closed.Store(true)
	sess.onExit()

	// It offers the protocol again and gets a session back.
	waitTestRecovery(t, "higher peer session restored", func() bool {
		return len(openTestSessions(higher.dex)) != 0
	})
}

// newTestRecoveryPair links two DEX nodes without forwarding and waits for a
// session on each.
func newTestRecoveryPair(t *testing.T, ctx context.Context) (*testRelayNode, *testRelayNode) {
	t.Helper()
	le := logrus.NewEntry(logrus.New())
	protocolContext := []byte("shared-object")
	a := newTestRelayNode(t, ctx, le, "a", protocolContext, 0)
	b := newTestRelayNode(t, ctx, le, "b", protocolContext, 0)
	connectTestRelayNodes(t, ctx, a, b)
	waitTestRecovery(t, "initial sessions", func() bool {
		return len(openTestSessions(a.dex)) != 0 && len(openTestSessions(b.dex)) != 0
	})
	return a, b
}

// testRelayPeerID returns the transport peer ID of a test node.
func testRelayPeerID(t *testing.T, ctx context.Context, n *testRelayNode) peer.ID {
	t.Helper()
	tpt, err := n.transport.GetTransport(ctx)
	if err != nil {
		t.Fatal(err)
	}
	return tpt.GetPeerID()
}

// putTestRecoveryBlock stores data in the node's own bucket.
func putTestRecoveryBlock(t *testing.T, ctx context.Context, n *testRelayNode, data []byte) *block.BlockRef {
	t.Helper()
	bkt, _, bktRef, err := bucket.ExBuildBucketAPI(ctx, n.tb.Bus, false, n.bucketID, n.tb.Volume.GetID(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(bktRef.Release)
	ref, _, err := bkt.GetBucket().PutBlock(ctx, data, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// openTestSessions returns the controller's sessions that are still open.
func openTestSessions(c *Controller) []*peerSession {
	var open []*peerSession
	for _, sess := range c.snapshotSessions() {
		if !sess.closed.Load() {
			open = append(open, sess)
		}
	}
	return open
}

// waitTestRecovery waits up to recoveryWait for fn.
func waitTestRecovery(t *testing.T, name string, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(recoveryWait)
	for !fn() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", name)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
