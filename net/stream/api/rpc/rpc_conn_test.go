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
	if r.recvRelease != nil {
		select {
		case r.recvStarted <- struct{}{}:
		default:
		}
		<-r.recvRelease
	}

	r.recvMtx.Lock()
	defer r.recvMtx.Unlock()

	r.recvCalls++
	if len(r.packets) == 0 {
		return nil, io.EOF
	}
	packet := r.packets[0]
	r.packets = r.packets[1:]
	return packet, nil
}

func TestNetConnConcurrentReadsConsumePacketOnce(t *testing.T) {
	want := []byte("hello world")
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	rpc := &rpcConnTestRPC{
		packets:     []*Data{{Data: want}},
		recvStarted: started,
		recvRelease: release,
	}
	conn := NewNetConn("", "", rpc)
	results := make(chan struct {
		n    int
		data byte
		err  error
	}, len(want))
	start := make(chan struct{})
	for range want {
		go func() {
			<-start
			var buffer [1]byte
			n, err := conn.Read(buffer[:])
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
	close(start)
	<-started
	close(release)

	got := make([]byte, 0, len(want))
	for range want {
		result := <-results
		if result.n != 1 || result.err != nil {
			t.Fatalf("concurrent read = %d/%v, want 1/nil", result.n, result.err)
		}
		got = append(got, result.data)
	}
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
	rpc := &rpcConnTestRPC{packets: []*Data{{Data: []byte("hello")}}}
	conn := NewNetConn("", "", rpc)
	buffer := make([]byte, 2)

	if n, err := conn.Read(nil); n != 0 || err != nil {
		t.Fatalf("zero-length read = %d/%v, want 0/nil", n, err)
	}
	for i, want := range []string{"he", "ll", "o"} {
		n, err := conn.Read(buffer)
		if err != nil || string(buffer[:n]) != want {
			t.Fatalf("read %d = %q/%v, want %q/nil", i, buffer[:n], err, want)
		}
	}
	if rpc.recvCalls != 1 {
		t.Fatalf("Recv calls = %d, want 1 while draining packet", rpc.recvCalls)
	}
}

func TestNetConnCloseForwardsToRPC(t *testing.T) {
	wantErr := errors.New("rpc closed")
	rpc := &rpcConnTestRPC{closeErr: wantErr}
	conn := NewNetConn("", "", rpc)

	if err := conn.Close(); !errors.Is(err, wantErr) {
		t.Fatalf("Close error = %v, want %v", err, wantErr)
	}
	if !rpc.closed {
		t.Fatal("Close did not reach RPC")
	}
}
