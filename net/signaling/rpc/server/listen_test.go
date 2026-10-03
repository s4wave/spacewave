package signaling_rpc_server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/net/peer"
	signaling "github.com/s4wave/spacewave/net/signaling/rpc"
	"github.com/sirupsen/logrus"
)

// testListenStream is a minimal Listen stream for tests.
type testListenStream struct {
	signaling.SRPCSignaling_ListenStream
	ctx context.Context
}

// Context returns the stream context.
func (s *testListenStream) Context() context.Context { return s.ctx }

// Send discards the response.
func (s *testListenStream) Send(*signaling.ListenResponse) error { return nil }

// TestListenRetainsPeerTracker verifies an attached Listen keeps its peer
// tracker when the last session want is removed.
func TestListenRetainsPeerTracker(t *testing.T) {
	// Start an authenticated signaling listener whose peer tracker must remain attached.
	pid := peer.ID("listener")
	server := NewServerWithIdentify(logrus.NewEntry(logrus.New()), func(context.Context) (peer.ID, error) {
		return pid, nil
	})
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() {
		done <- server.Listen(&signaling.ListenRequest{}, &testListenStream{ctx: ctx})
	}()

	// Wait for the listener to attach.
	deadline := time.Now().Add(2 * time.Second)
	for {
		server.mtx.Lock()
		tkr := server.peers[pid.String()]
		listening := tkr != nil && tkr.listening
		server.mtx.Unlock()
		if listening {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for listener to attach")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Verify that an attached signaling listener retains its peer tracker.
	server.mtx.Lock()
	released := server.maybeReleasePeer(pid.String())
	server.mtx.Unlock()
	if released {
		t.Fatal("listening peer tracker was released")
	}

	// Stop the signaling listener and wait for its stream to return.
	cancel()
	select {
	case err := <-done:
		if err != nil && !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out stopping listener")
	}

	// Verify that listener shutdown releases its peer tracker.
	server.mtx.Lock()
	_, exists := server.peers[pid.String()]
	server.mtx.Unlock()
	if exists {
		t.Fatal("expected peer tracker released after listener exit")
	}
}
