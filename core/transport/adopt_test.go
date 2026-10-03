package transport_test

import (
	"context"
	"crypto/rand"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/s4wave/spacewave/testbed"
	"github.com/sirupsen/logrus"
)

func TestSessionTransportAdoptsAuthenticatedLink(t *testing.T) {
	// Start the session transport testbed with a bounded lifetime.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	tb, err := testbed.Default(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tb.Release()

	// Generate private keys for the local and remote link peers.
	localKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	remoteKey, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}

	// Derive the peer IDs used by the authenticated test link.
	localPeer, err := peer.IDFromPrivateKey(localKey)
	if err != nil {
		t.Fatal(err)
	}
	remotePeer, err := peer.IDFromPrivateKey(remoteKey)
	if err != nil {
		t.Fatal(err)
	}

	// Start the session transport and wait for its local controllers.
	owner, err := transport.NewSessionTransport(logrus.NewEntry(logrus.New()), tb.Bus, localKey, "", "")
	if err != nil {
		t.Fatal(err)
	}
	executeDone := make(chan error, 1)
	go func() { executeDone <- owner.Execute(ctx) }()
	if err := owner.AwaitReady(ctx); err != nil {
		t.Fatal(err)
	}

	// Adopt the authenticated link into the running session transport.
	lnk := newAdoptTestLink(localPeer, remotePeer)
	if err := owner.AdoptLink(ctx, lnk); err != nil {
		t.Fatal(err)
	}

	// Wait for the adopted link to appear in the transport snapshot.
	for {
		links, waits := owner.GetLinkSnapshotsWithWait()
		if len(links) == 1 && links[0].RemotePeerID == remotePeer {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatal("adopted link did not enter the Session transport")
		case <-waitAny(waits):
		}
	}

	// Verify that session transport shutdown closes the adopted link.
	cancel()
	select {
	case <-lnk.closed:
	case <-time.After(time.Second):
		t.Fatal("Session transport shutdown did not close the adopted link")
	}
	<-executeDone
}

func waitAny(channels []<-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	var once sync.Once
	for _, channel := range channels {
		go func() {
			select {
			case <-channel:
				once.Do(func() { close(done) })
			case <-done:
			}
		}()
	}
	return done
}

type adoptTestLink struct {
	localPeer  peer.ID
	remotePeer peer.ID
	closed     chan struct{}
}

func newAdoptTestLink(localPeer, remotePeer peer.ID) *adoptTestLink {
	return &adoptTestLink{localPeer: localPeer, remotePeer: remotePeer, closed: make(chan struct{})}
}

func (l *adoptTestLink) GetUUID() uint64                { return 2 }
func (l *adoptTestLink) GetTransportUUID() uint64       { return 1 }
func (l *adoptTestLink) GetRemoteTransportUUID() uint64 { return 3 }
func (l *adoptTestLink) GetRemotePeer() peer.ID         { return l.remotePeer }
func (l *adoptTestLink) GetLocalPeer() peer.ID          { return l.localPeer }
func (l *adoptTestLink) OpenStream(stream.OpenOpts) (stream.Stream, error) {
	return nil, io.EOF
}

func (l *adoptTestLink) AcceptStream() (stream.Stream, stream.OpenOpts, error) {
	<-l.closed
	return nil, stream.OpenOpts{}, io.EOF
}

func (l *adoptTestLink) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}
