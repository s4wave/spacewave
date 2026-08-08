package transport_controller

import (
	"context"
	"io"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	cbc "github.com/aperturerobotics/controllerbus/core"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/sirupsen/logrus"
)

func TestLinkLossBroadcastsSnapshotWaiters(t *testing.T) {
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	b, _, err := cbc.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}

	localPeer := peer.ID("local-peer")
	remotePeer := peer.ID("remote-peer")
	tpt := &testTransport{peerID: localPeer}
	c := NewController(
		le,
		b,
		controller.NewInfo("test", controller.MustParseVersion("0.0.0"), "test"),
		localPeer,
		false,
		nil,
	)
	execCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		c.execCtx = execCtx
		c.peerID = localPeer
		c.tpt = tpt
		broadcast()
	})

	h := newTransportHandler(ctx, c)
	_ = h.tpt.SetResult(tpt, nil)
	_, waitEstablished := c.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{remotePeer})

	lnk := newTestLink(localPeer, remotePeer)
	h.HandleLinkEstablished(lnk)
	assertClosed(t, waitEstablished)

	linked, waitLost := c.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{remotePeer})
	if _, ok := linked[remotePeer]; !ok {
		t.Fatal("expected remote peer to be linked")
	}
	if err := lnk.Close(); err != nil {
		t.Fatal(err.Error())
	}
	h.HandleLinkLost(lnk)
	assertClosed(t, waitLost)

	linked, _ = c.GetLinkedPeerIDsSnapshotWithWait([]peer.ID{remotePeer})
	if _, ok := linked[remotePeer]; ok {
		t.Fatal("expected remote peer to be unlinked")
	}
}

func assertClosed(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	default:
		t.Fatal("expected wait channel to close")
	}
}

type testTransport struct {
	peerID peer.ID
}

func (t *testTransport) Execute(ctx context.Context) error {
	<-ctx.Done()
	return ctx.Err()
}

func (t *testTransport) GetUUID() uint64 {
	return 1
}

func (t *testTransport) GetPeerID() peer.ID {
	return t.peerID
}

func (t *testTransport) Close() error {
	return nil
}

type testLink struct {
	localPeer  peer.ID
	remotePeer peer.ID
	closed     chan struct{}
}

func newTestLink(localPeer, remotePeer peer.ID) *testLink {
	return &testLink{
		localPeer:  localPeer,
		remotePeer: remotePeer,
		closed:     make(chan struct{}),
	}
}

func (l *testLink) GetUUID() uint64 {
	return 2
}

func (l *testLink) GetTransportUUID() uint64 {
	return 1
}

func (l *testLink) OpenStream(opts stream.OpenOpts) (stream.Stream, error) {
	return nil, io.EOF
}

func (l *testLink) AcceptStream() (stream.Stream, stream.OpenOpts, error) {
	<-l.closed
	return nil, stream.OpenOpts{}, io.EOF
}

func (l *testLink) GetRemotePeer() peer.ID {
	return l.remotePeer
}

func (l *testLink) GetLocalPeer() peer.ID {
	return l.localPeer
}

func (l *testLink) GetRemoteTransportUUID() uint64 {
	return 3
}

func (l *testLink) Close() error {
	select {
	case <-l.closed:
	default:
		close(l.closed)
	}
	return nil
}

var (
	_ transport.Transport = (*testTransport)(nil)
	_ link.Link           = (*testLink)(nil)
)
