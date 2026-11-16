package rwc

import (
	"context"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/peer"
)

// TestPacketConn tests the packet conn.
func TestPacketConn(t *testing.T) {
	// Prepare the connection test context.
	ctx := context.Background()

	// Create the first peer for the local connection address.
	peer1, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the second peer for the remote connection address.
	peer2, err := peer.NewPeer(nil)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Connect the peer addresses with an in-memory stream pair.
	a1, a2 := peer.NewNetAddr(peer1.GetPeerID()), peer.NewNetAddr(peer2.GetPeerID())
	c1, c2 := net.Pipe()

	// Wrap both stream ends with bounded packet connections.
	maxPacketSize := uint32(1500)
	pc1 := NewPacketConn(ctx, c1, a1, a2, maxPacketSize, 10)
	pc2 := NewPacketConn(ctx, c2, a2, a1, maxPacketSize, 10)

	// Send the test payload and require a complete write.
	data := []byte("testing 1234")
	n, err := pc1.WriteTo(data, pc2.LocalAddr())
	if err == nil && n != len(data) {
		err = errors.Errorf("expected to write %d but wrote %d", len(data), n)
	}
	if err != nil {
		t.Fatal(err.Error())
	}

	// Receive the payload through the remote connection.
	outData := make([]byte, len(data)*2)
	on, oa, err := pc2.ReadFrom(outData)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify that the received payload has the expected length.
	outData = outData[:on]
	if on != len(data) {
		t.Fatalf(
			"length incorrect received %v != %v data: %v",
			on,
			len(data),
			string(outData),
		)
	}

	// Verify that the packet retains the sending peer address.
	outAddr := oa.String()
	expectedAddr := a1.String()
	if outAddr != expectedAddr {
		t.Fatalf(
			"expected remote addr %s but got %s",
			expectedAddr,
			outAddr,
		)
	}
}

// tempReadErr is a temporary net.Error like the dtls deadline error pion sctp
// leaves on a closed data channel.
type tempReadErr struct{}

func (tempReadErr) Error() string   { return "deadline exceeded" }
func (tempReadErr) Timeout() bool   { return true }
func (tempReadErr) Temporary() bool { return true }

// tempErrRwc fails every read with tempReadErr.
type tempErrRwc struct{ io.ReadWriteCloser }

func (tempErrRwc) Read([]byte) (int, error) { return 0, tempReadErr{} }

// TestRwcPacketConnReadErrNotTemporary checks that a read error is not a
// net.Error, so quic-go closes the transport instead of retrying forever.
func TestRwcPacketConnReadErrNotTemporary(t *testing.T) {
	// Read once from a conn whose reader fails with a temporary error.
	pc := NewRwcPacketConn(tempErrRwc{}, nil, nil)
	_, _, err := pc.ReadFrom(make([]byte, 16))

	// Require a wrapped error that is no longer a net.Error.
	if err == nil {
		t.Fatal("expected read error")
	}
	if _, ok := err.(net.Error); ok {
		t.Fatalf("read error %v is a net.Error", err)
	}
	if !errors.Is(err, tempReadErr{}) {
		t.Fatalf("read error %v does not wrap the cause", err)
	}
}

// deadlineReadContext reports when a read reaches its wait after subscribing.
type deadlineReadContext struct {
	context.Context
	waiting chan struct{}
}

// Done reports the read's wait without delaying cancellation.
func (c deadlineReadContext) Done() <-chan struct{} {
	// Signal each wait without blocking a read or a repeated deadline wakeup.
	select {
	case c.waiting <- struct{}{}:
	default:
	}
	return c.Context.Done()
}

// TestReadDeadline checks deadline changes after a reader has subscribed.
func TestReadDeadline(t *testing.T) {
	// Wait for the reader's select after it snapshots its deadline.
	var deadline readDeadline
	waiting := make(chan struct{}, 1)
	ctx := deadlineReadContext{Context: t.Context(), waiting: waiting}
	packets := make(chan []byte)
	result := make(chan error, 1)
	read := func() { _, _, err := deadline.read(ctx, packets); result <- err }
	go read()
	<-waiting

	// Setting an expired deadline must wake a read that started without one.
	deadline.set(time.Now().Add(-time.Second))
	if err := <-result; !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("pending read returned %v, want deadline exceeded", err)
	}
	if _, _, err := deadline.read(ctx, packets); !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("future read returned %v, want deadline exceeded", err)
	}

	// Clearing a future deadline must wake the reader to discard its timer.
	deadline.set(time.Now().Add(time.Hour))
	go read()
	<-waiting
	deadline.set(time.Time{})
	<-waiting
	packets <- []byte{1}
	if err := <-result; err != nil {
		t.Fatal(err)
	}

	// A future deadline expires through the timer without another setter.
	deadline.set(time.Now().Add(10 * time.Millisecond))
	go read()
	if err := <-result; !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("timed read returned %v, want deadline exceeded", err)
	}
	for len(waiting) != 0 {
		<-waiting
	}

	// Extending an expired deadline allows the next packet to arrive.
	deadline.set(time.Now().Add(-time.Second))
	deadline.set(time.Now().Add(time.Hour))
	go read()
	<-waiting
	packets <- []byte{2}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}

// TestConnDeadlineConcurrent checks both deadline setters against pending reads
// on the framed and buffered connection adapters.
func TestConnDeadlineConcurrent(t *testing.T) {
	// Exercise both connection implementations through their deadline methods.
	for _, packet := range []bool{true, false} {
		name := "Conn"
		if packet {
			name = "PacketConn"
		}
		t.Run(name, func(t *testing.T) {
			// Hold a read pending on an otherwise idle in-memory stream.
			stream, remote := net.Pipe()
			defer remote.Close()
			var conn interface {
				SetDeadline(time.Time) error
				SetReadDeadline(time.Time) error
				Close() error
			}
			result := make(chan error, 1)
			if packet {
				pc := NewPacketConn(t.Context(), stream, nil, nil, 1500, 1)
				conn = pc
				go func() { _, _, err := pc.ReadFrom(make([]byte, 1)); result <- err }()
			} else {
				c := NewConn(t.Context(), stream, nil, nil, 1)
				conn = c
				go func() { _, err := c.Read(make([]byte, 1)); result <- err }()
			}
			defer conn.Close()

			// Exercise both setters concurrently before timing out the read.
			done := make(chan struct{}, 2)
			for _, set := range []func(time.Time) error{conn.SetDeadline, conn.SetReadDeadline} {
				go func() {
					for range 1000 {
						_ = set(time.Time{})
					}
					done <- struct{}{}
				}()
			}
			<-done
			<-done
			if err := conn.SetReadDeadline(time.Now().Add(-time.Second)); err != nil {
				t.Fatal(err)
			}
			if err := <-result; !errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatalf("pending read returned %v, want deadline exceeded", err)
			}
		})
	}
}
