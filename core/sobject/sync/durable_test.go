package sobject_sync

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/net/peer"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
)

// TestWriteMessagesWaitsDurable checks that no frame reaches the peer before
// the host reports the local state durable.
func TestWriteMessagesWaitsDurable(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()

	// Build a host whose durability wait blocks until released.
	const soID = "durable-writer"
	owner, reader := mustKeyPair(t), mustKeyPair(t)
	localID, err := peer.IDFromPrivateKey(owner)
	if err != nil {
		t.Fatal(err)
	}
	remote, err := peer.IDFromPrivateKey(reader)
	if err != nil {
		t.Fatal(err)
	}
	ctr := ccontainer.NewCContainerVT(authenticationState(t, soID, owner, reader))
	watch := func(context.Context, string, func()) (ccontainer.Watchable[*sobject.SOState], func(), error) {
		return ctr, func() {}, nil
	}
	waiting, durable := make(chan struct{}), make(chan struct{})
	host := sobject.NewSOHost(ctx, watch, nil, soID, &sobject.SOHostSyncFuncs{
		WaitDurable: func(ctx context.Context) error {
			close(waiting)
			select {
			case <-durable:
				return nil
			case <-ctx.Done():
				return context.Cause(ctx)
			}
		},
	})
	t.Cleanup(host.ClearContext)
	local := &SOSync{localObjectPeerID: localID, soHost: host}

	// Start the writer over a real pipe.
	left, right := net.Pipe()
	t.Cleanup(func() { left.Close(); right.Close() })
	observed := &authenticationStream{Conn: left, messages: make(chan *SOSyncMessage, 1)}
	outbound, sent := make(chan *SOSyncMessage), make(chan error, 1)
	done := make(chan error, 1)
	go func() {
		done <- local.writeMessages(ctx, stream_packet.NewSession(observed, maxMessageSize), remote, ctr, outbound, sent)
	}()
	t.Cleanup(func() { cancel(); <-done })

	// The frame waits in WaitDurable and has not been written.
	outbound <- leanSyncWriterMessage(1)
	select {
	case <-waiting:
	case <-ctx.Done():
		t.Fatal("writer did not wait for durability")
	}
	select {
	case <-observed.messages:
		t.Fatal("frame written before the state was durable")
	default:
	}

	// Once durable, the frame reaches the peer.
	close(durable)
	received := &SOSyncMessage{}
	if err := stream_packet.NewSession(right, maxMessageSize).RecvMsg(received); err != nil {
		t.Fatal(err)
	}
	if leanSyncWriterKind(received) != 1 {
		t.Fatalf("received frame kind %d, want snapshot", leanSyncWriterKind(received))
	}
	if err := <-sent; err != nil {
		t.Fatal(err)
	}
}
