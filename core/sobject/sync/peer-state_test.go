package sobject_sync

import (
	"bytes"
	"context"
	"net"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
)

// TestPeerStateReportsAdoptedState proves the held-state observer sees the
// digest of the sender's state once the receiver adopts it, so a sender can
// wait for a peer to hold its writes before retiring its transport.
func TestPeerStateReportsAdoptedState(t *testing.T) {
	// An owner and a reader start from the same authenticated state.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	const soID = "peer-state-adopted"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	initial := authenticationState(t, soID, owner, reader)
	local := newAuthenticationPeer(t, soID, owner, initial)
	remote := newAuthenticationPeer(t, soID, reader, initial)
	readerID, err := peer.IDFromPrivateKey(reader)
	if err != nil {
		t.Fatal(err)
	}
	reported := make(chan []byte, 16)
	local.SetPeerStateObserver(func(remoteID peer.ID, stateHash []byte) {
		if remoteID != readerID {
			t.Errorf("state reported for %s, want %s", remoteID, readerID)
		}
		reported <- stateHash
	})

	// The owner writes while the reader is offline.
	if err := local.soHost.UpdateSOState(ctx, func(state *sobject.SOState) error {
		writeSyncOp(t, soID, state, owner, "write the reader must hold")
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	target, err := local.soHost.GetHostState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want, err := StateHash(target)
	if err != nil {
		t.Fatal(err)
	}
	before, err := StateHash(initial)
	if err != nil {
		t.Fatal(err)
	}

	// The reader first advertises its own older state, then the adopted one.
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	go func() { _ = local.runStream(ctx, gateLogger(), left, "transport-a", "transport-b") }()
	go func() { _ = remote.runStream(ctx, gateLogger(), right, "transport-b", "transport-a") }()
	var sawBefore bool
	for {
		select {
		case sum := <-reported:
			if bytes.Equal(sum, before) {
				sawBefore = true
				continue
			}
			if !bytes.Equal(sum, want) {
				t.Fatal("reader reported a state that is neither its own nor the adopted one")
			}
			if !sawBefore {
				t.Fatal("reader reported the adopted state before its own")
			}
			return
		case <-ctx.Done():
			t.Fatal("reader never reported holding the owner's state")
		case <-time.After(5 * time.Second):
			t.Fatal("reader never reported holding the owner's state")
		}
	}
}
