package stream_api_rpc

import (
	"context"
	"errors"
	"io"
	"slices"
	"sync"
	"testing"
)

type rpcConnTestRPC struct {
	packets     []*Data
	closeErr    error
	recvCalls   int
	closed      bool
	recvMtx     sync.Mutex
	recvStarted chan struct{}
	recvRelease <-chan struct{}
}

func (r *rpcConnTestRPC) Context() context.Context {
	return context.Background()
}

func (*rpcConnTestRPC) Send(*Data) error {
	return nil
}

func (r *rpcConnTestRPC) Recv() (*Data, error) {
	// Hold the RPC receive until the test releases its packet.
	if r.recvRelease != nil {
		select {
		case r.recvStarted <- struct{}{}:
		default:
		}
		<-r.recvRelease
	}

	// Serialize packet access and receive-call accounting.
	r.recvMtx.Lock()
	defer r.recvMtx.Unlock()

	// Count the receive and consume the next queued RPC packet.
	r.recvCalls++
	if len(r.packets) == 0 {
		return nil, io.EOF
	}
	packet := r.packets[0]
	r.packets = r.packets[1:]
	return packet, nil
}

func TestNetConnConcurrentReadsConsumePacketOnce(t *testing.T) {
	// Prepare one RPC packet and gates for concurrent connection reads.
	want := []byte("hello world")
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	rpc := &rpcConnTestRPC{
		packets:     []*Data{{Data: want}},
		recvStarted: started,
		recvRelease: release,
	}
	conn := NewNetConn("", "", rpc)

	// Collect one result for each reader of the shared connection.
	results := make(chan struct {
		n    int
		data byte
		err  error
	}, len(want))

	// Launch one gated connection reader per packet byte.
	start := make(chan struct{})
	for range want {
		go func() {
			// Read one packet byte after all connection readers are ready.
			<-start
			var buffer [1]byte
			n, err := conn.Read(buffer[:])

			// Send the connection read result to the assertion loop.
			result := struct {
				n    int
				data byte
				err  error
			}{n: n, err: err}
			if n > 0 {
				result.data = buffer[0]
			}
			results <- result
		}()
	}

	// Release the RPC packet after the concurrent reads have started.
	close(start)
	<-started
	close(release)

	// Require every concurrent connection read to return one byte.
	got := make([]byte, 0, len(want))
	for range want {
		result := <-results
		if result.n != 1 || result.err != nil {
			t.Fatalf("concurrent read = %d/%v, want 1/nil", result.n, result.err)
		}
		got = append(got, result.data)
	}

	// Verify the connection delivered every packet byte exactly once.
	expected := slices.Clone(want)
	slices.Sort(got)
	slices.Sort(expected)
	if !slices.Equal(got, expected) {
		t.Fatalf("concurrent reads = %q, want each byte of %q exactly once", got, want)
	}
}

func (r *rpcConnTestRPC) Close() error {
	r.closed = true
	return r.closeErr
}

func TestNetConnReadRetainsUnreadPacketData(t *testing.T) {
	// Prepare a connection whose read buffer is smaller than its RPC packet.
	rpc := &rpcConnTestRPC{packets: []*Data{{Data: []byte("hello")}}}
	conn := NewNetConn("", "", rpc)
	buffer := make([]byte, 2)

	// Verify an empty connection read leaves the RPC packet untouched.
	if n, err := conn.Read(nil); n != 0 || err != nil {
		t.Fatalf("zero-length read = %d/%v, want 0/nil", n, err)
	}

	// Drain the RPC packet across successive short connection reads.
	for i, want := range []string{"he", "ll", "o"} {
		// Read the next buffered segment of the RPC packet.
		n, err := conn.Read(buffer)

		// Verify the short connection read matches the expected segment.
		if err != nil || string(buffer[:n]) != want {
			t.Fatalf("read %d = %q/%v, want %q/nil", i, buffer[:n], err, want)
		}
	}

	// Verify the connection fetched the buffered packet only once.
	if rpc.recvCalls != 1 {
		t.Fatalf("Recv calls = %d, want 1 while draining packet", rpc.recvCalls)
	}
}

func TestNetConnCloseForwardsToRPC(t *testing.T) {
	// Prepare an RPC whose close returns a recognizable error.
	wantErr := errors.New("rpc closed")
	rpc := &rpcConnTestRPC{closeErr: wantErr}
	conn := NewNetConn("", "", rpc)

	// Verify the connection returns the RPC close error.
	if err := conn.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("Close error = %v, want %v", err, wantErr)
	}

	// Verify closing the connection reached the RPC.
	if !rpc.closed {
		t.Fatal("Close did not reach RPC")
	}
}
