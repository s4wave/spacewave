package signaling_rpc_client

import (
	"context"
	"crypto/rand"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/crypto"
)

// TestSendSurvivesSessionReopen keeps an outstanding message owned by its
// sender when the relay opens a new session before acknowledging it.
func TestSendSurvivesSessionReopen(t *testing.T) {
	privKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	client := &Client{privKey: privKey}
	tracker := &clientPeerTracker{}
	ref := &ClientPeerRef{c: client, tkr: tracker}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	// Open the first signaling session and start a send.
	first := uint64(1)
	tracker.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		tracker.open = &first
		broadcast()
	})
	result := make(chan error, 1)
	go func() {
		_, err := ref.Send(ctx, []byte("offer"))
		result <- err
	}()

	// Wait for the sender to queue its message in the first session.
	var queuedSeqno uint64
	for queuedSeqno == 0 {
		var waitCh <-chan struct{}
		tracker.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if tracker.out != nil {
				queuedSeqno = tracker.out.Seqno
			}
			waitCh = getWaitCh()
		})
		if queuedSeqno != 0 {
			break
		}
		select {
		case <-waitCh:
		case <-time.After(2 * time.Second):
			t.Fatal("send did not queue its message")
		}
	}

	// Reopen before the acknowledgement, then acknowledge the same message.
	second := uint64(2)
	tracker.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		tracker.open = &second
		tracker.outSent = false
		broadcast()
	})
	tracker.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		tracker.outAcked = true
		broadcast()
	})
	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("send after session reopen: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("send lost ownership of its message after session reopen")
	}
}
