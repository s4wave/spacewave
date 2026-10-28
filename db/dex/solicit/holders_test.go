package dex_solicit

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestStoreHoldersMissIsNotFound proves a read is a miss once each holder
// answered that it lacks the block, even when a peer that is not a holder
// failed to answer.
func TestStoreHoldersMissIsNotFound(t *testing.T) {
	// Connect two holders and a peer that is not a holder.
	ctx := t.Context()
	c := newTestDexSolicitController()
	c.SetHolders([]peer.ID{"holder-a", "holder-b"})
	ref := testDexBlockRef(t, "absent")
	a := addTestDexSession(t, ctx, c, "holder-a")
	b := addTestDexSession(t, ctx, c, "holder-b")
	other := addTestDexSession(t, ctx, c, "other")

	// Both holders lack the block; the other peer fails.
	answers := []<-chan error{
		answerTestDexMiss(a),
		answerTestDexMiss(b),
		answerTestDexError(other, "other failed"),
	}
	_, found, err := NewStore(c).GetBlock(ctx, ref)
	if err != nil || found {
		t.Fatalf("read = found %v, %v; want a miss", found, err)
	}
	for _, answer := range answers {
		if err := recvTestDexValue(t, answer, "peer answer"); err != nil {
			t.Fatal(err)
		}
	}
}

// TestStoreHoldersOfflineIsUnavailable proves a read that could not ask a
// holder reports the block unavailable and names that holder.
func TestStoreHoldersOfflineIsUnavailable(t *testing.T) {
	// Connect one of two holders, which lacks the block.
	ctx := t.Context()
	c := newTestDexSolicitController()
	c.SetHolders([]peer.ID{"holder-a", "holder-c"})
	ref := testDexBlockRef(t, "held-offline")
	answer := answerTestDexMiss(addTestDexSession(t, ctx, c, "holder-a"))

	// The holder that is offline was not asked.
	_, found, err := NewStore(c).GetBlock(ctx, ref)
	assertTestDexUnavailable(t, found, err, ref, "holder-c")
	if err := recvTestDexValue(t, answer, "holder answer"); err != nil {
		t.Fatal(err)
	}
}

// TestStoreHoldersExistsIgnoresOffline proves an existence check takes the
// connected peers' answer, so a holder that is offline does not fail it.
func TestStoreHoldersExistsIgnoresOffline(t *testing.T) {
	// Connect one of two holders, which lacks the block.
	ctx := t.Context()
	c := newTestDexSolicitController()
	c.SetHolders([]peer.ID{"holder-a", "holder-c"})
	answer := answerTestDexMiss(addTestDexSession(t, ctx, c, "holder-a"))

	// The check reports the block absent without naming the offline holder.
	found, err := NewStore(c).GetBlockExists(ctx, testDexBlockRef(t, "probe-offline"))
	if err != nil || found {
		t.Fatalf("exists = %v, %v; want false", found, err)
	}
	if err := recvTestDexValue(t, answer, "holder answer"); err != nil {
		t.Fatal(err)
	}
}

// TestStoreHoldersFailedIsUnavailable proves a holder whose request failed
// leaves the read unavailable, naming the holder and carrying the failure.
func TestStoreHoldersFailedIsUnavailable(t *testing.T) {
	// Connect the only holder, whose request fails.
	ctx := t.Context()
	c := newTestDexSolicitController()
	c.SetHolders([]peer.ID{"holder-a"})
	ref := testDexBlockRef(t, "held-failed")
	answer := answerTestDexError(addTestDexSession(t, ctx, c, "holder-a"), "lookup failed")

	// The failure is the cause of the unavailable read.
	_, found, err := NewStore(c).GetBlock(ctx, ref)
	assertTestDexUnavailable(t, found, err, ref, "holder-a")
	var unavailable *block.UnavailableError
	if !errors.As(err, &unavailable) || unavailable.Cause == nil || unavailable.Cause.Error() != "lookup failed" {
		t.Fatalf("read cause = %v; want the holder's failure", err)
	}
	if err := recvTestDexValue(t, answer, "holder answer"); err != nil {
		t.Fatal(err)
	}
}

// TestStoreHoldersLinkedHolderAnswers proves a holder that never opens a link
// is answered by the session with the transport peer it proved its identity
// over, for as long as one such link stays up.
func TestStoreHoldersLinkedHolderAnswers(t *testing.T) {
	// Connect the transport holder and link a storage holder through it twice.
	ctx := t.Context()
	c := newTestDexSolicitController()
	c.SetHolders([]peer.ID{"transport-a", "storage-a"})
	ref := testDexBlockRef(t, "linked")
	remote := addTestDexSession(t, ctx, c, "transport-a")
	store := NewStore(c)
	c.LinkHolder("transport-a", "storage-a", true)
	c.LinkHolder("transport-a", "storage-a", true)

	// With one of the two links down, the transport still answers for both.
	c.LinkHolder("transport-a", "storage-a", false)
	answer := answerTestDexMiss(remote)
	if _, found, err := store.GetBlock(ctx, ref); err != nil || found {
		t.Fatalf("linked read = found %v, %v; want a miss", found, err)
	}
	if err := recvTestDexValue(t, answer, "linked answer"); err != nil {
		t.Fatal(err)
	}

	// With the last link down, the storage holder is unanswered.
	c.LinkHolder("transport-a", "storage-a", false)
	answer = answerTestDexMiss(remote)
	_, found, err := store.GetBlock(ctx, ref)
	assertTestDexUnavailable(t, found, err, ref, "storage-a")
	if err := recvTestDexValue(t, answer, "unlinked answer"); err != nil {
		t.Fatal(err)
	}
}

// TestStoreHoldersNoneIsNotFound proves a read whose holders are known to be
// none is a miss at once instead of waiting for a peer to connect.
func TestStoreHoldersNoneIsNotFound(t *testing.T) {
	// Bound the read so a wait for a peer fails the test.
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()

	// Read with an empty set of holders and no peer connected.
	c := newTestDexSolicitController()
	c.SetHolders([]peer.ID{})
	_, found, err := NewStore(c).GetBlock(ctx, testDexBlockRef(t, "solo"))
	if err != nil || found {
		t.Fatalf("read = found %v, %v; want a miss", found, err)
	}
}

// TestControllerSetHoldersBroadcastsChange proves setting the holders wakes
// waiting reads only when the set changes, keeping nil distinct from empty.
func TestControllerSetHoldersBroadcastsChange(t *testing.T) {
	c := newTestDexSolicitController()
	steps := []struct {
		holders []peer.ID
		changed bool
	}{
		{holders: nil, changed: false},
		{holders: []peer.ID{}, changed: true},
		{holders: []peer.ID{}, changed: false},
		{holders: []peer.ID{"a"}, changed: true},
		{holders: []peer.ID{"a"}, changed: false},
		{holders: nil, changed: true},
	}
	for i, step := range steps {
		// Capture the wait channel before the change.
		var wait <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			wait = getWait()
		})

		// Require a broadcast exactly when the set changed.
		c.SetHolders(step.holders)
		select {
		case <-wait:
			if !step.changed {
				t.Fatalf("step %d broadcast without a change", i)
			}
		default:
			if step.changed {
				t.Fatalf("step %d changed without a broadcast", i)
			}
		}
	}
}

// addTestDexSession starts a session with remoteID and registers it with c.
// It returns the remote end, which the test answers on.
func addTestDexSession(t *testing.T, ctx context.Context, c *Controller, remoteID peer.ID) *stream_packet.Session {
	// Start the session for the test's lifetime.
	t.Helper()
	sess, remote, cleanup := newTestPeerSessionPair(c, remoteID)
	t.Cleanup(cleanup)
	sess.start(ctx)

	// Register it as the newest session with remoteID.
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.sessions[remoteID.String()] = []*peerSession{sess}
		broadcast()
	})
	return remote
}

// answerTestDexMiss answers the next request on remote with a miss.
func answerTestDexMiss(remote *stream_packet.Session) <-chan error {
	return answerTestDex(remote, func(req *DexMessage) *DexMessage {
		return &DexMessage{RequestId: req.GetRequestId(), IsResponse: true}
	})
}

// answerTestDexError answers the next request on remote with a failure.
func answerTestDexError(remote *stream_packet.Session, msg string) <-chan error {
	return answerTestDex(remote, func(req *DexMessage) *DexMessage {
		return &DexMessage{RequestId: req.GetRequestId(), IsResponse: true, Error: msg}
	})
}

// answerTestDex answers the next request on remote with respond's message.
func answerTestDex(remote *stream_packet.Session, respond func(*DexMessage) *DexMessage) <-chan error {
	done := make(chan error, 1)
	go func() {
		var req DexMessage
		if err := remote.RecvMsg(&req); err != nil {
			done <- err
			return
		}
		done <- remote.SendMsg(respond(&req))
	}()
	return done
}

// assertTestDexUnavailable requires an unavailable read of ref naming holders.
func assertTestDexUnavailable(t *testing.T, found bool, err error, ref *block.BlockRef, holderIDs ...peer.ID) {
	// Name the holders as the error does.
	t.Helper()
	holders := make([]string, len(holderIDs))
	for i, id := range holderIDs {
		holders[i] = id.String()
	}

	// Require an unavailable read naming exactly those holders.
	if found || !errors.Is(err, block.ErrUnavailable) || errors.Is(err, block.ErrNotFound) {
		t.Fatalf("read = found %v, %v; want unavailable", found, err)
	}
	var unavailable *block.UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("read error %T does not name the holders", err)
	}
	if !unavailable.Ref.EqualsRef(ref) || !slices.Equal(unavailable.Holders, holders) {
		t.Fatalf("unavailable = %s %v; want %s %v", unavailable.Ref.MarshalString(), unavailable.Holders, ref.MarshalString(), holders)
	}
}
