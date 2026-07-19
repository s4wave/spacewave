//go:build !js

package resource_listener

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource "github.com/s4wave/spacewave/bldr/resource"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
)

const (
	listenerTestServiceID     = "test.listener"
	listenerTestPingMethodID  = "Ping"
	listenerTestWatchMethodID = "Watch"
)

func TestHandoffBlocksActiveListener(t *testing.T) {
	const socketPath = "/tmp/spacewave.sock"
	tests := []struct {
		name    string
		handoff yield_policy.HandoffState
		want    bool
	}{
		{
			name: "inactive",
		},
		{
			name: "matching socket",
			handoff: yield_policy.HandoffState{
				Active:     true,
				SocketPath: socketPath,
			},
			want: true,
		},
		{
			name: "unscoped process handoff",
			handoff: yield_policy.HandoffState{
				Active: true,
			},
			want: true,
		},
		{
			name: "different socket",
			handoff: yield_policy.HandoffState{
				Active:     true,
				SocketPath: "/tmp/other.sock",
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := handoffBlocksListener(tt.handoff); got != tt.want {
				t.Fatalf("handoffBlocksListener() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAcceptCountingListenerKeepsExistingClientAfterPeerDeparts(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		cancel()
		t.Fatal(err)
	}

	status := NewStatusBroker()
	status.SetListening(true)
	watchNext := make(chan struct{})
	mux := srpc.NewMux(srpc.InvokerFunc(func(serviceID, methodID string, strm srpc.Stream) (bool, error) {
		if serviceID != listenerTestServiceID {
			return false, nil
		}
		switch methodID {
		case listenerTestPingMethodID:
			if err := strm.MsgRecv(new(resource.ResourceRefReleaseRequest)); err != nil {
				return true, err
			}
			return true, strm.MsgSend(new(resource.ResourceRefReleaseResponse))
		case listenerTestWatchMethodID:
			if err := strm.MsgRecv(new(resource.ResourceRefReleaseRequest)); err != nil {
				return true, err
			}
			if err := strm.MsgSend(new(resource.ResourceRefReleaseResponse)); err != nil {
				return true, err
			}
			select {
			case <-watchNext:
			case <-strm.Context().Done():
				return true, context.Canceled
			}
			return true, strm.MsgSend(new(resource.ResourceRefReleaseResponse))
		default:
			return false, nil
		}
	}))
	serverErr := make(chan error, 1)
	go func() {
		serverErr <- acceptCountingListener(ctx, listener, srpc.NewServer(mux), status)
	}()
	defer func() {
		cancel()
		_ = listener.Close()
		select {
		case <-serverErr:
		case <-time.After(5 * time.Second):
			t.Error("listener did not stop")
		}
	}()

	clientA, closeA := dialListenerTestClient(t, listener.Addr().String())
	defer closeA()
	watchCtx, cancelWatch := context.WithCancel(t.Context())
	defer cancelWatch()
	watchA, err := clientA.NewStream(
		watchCtx,
		listenerTestServiceID,
		listenerTestWatchMethodID,
		new(resource.ResourceRefReleaseRequest),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer watchA.Close()
	recvListenerTestWatch(t, watchA)

	clientB, closeB := dialListenerTestClient(t, listener.Addr().String())
	pingListenerTestClient(t, clientB)
	waitListenerClientCount(t, status, 2)

	closeB()
	waitListenerClientCount(t, status, 1)
	close(watchNext)
	recvListenerTestWatch(t, watchA)

	closeA()
	waitListenerClientCount(t, status, 0)
}

func dialListenerTestClient(t *testing.T, address string) (srpc.Client, func()) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	muxed, err := srpc.NewMuxedConn(conn, true, nil)
	if err != nil {
		_ = conn.Close()
		t.Fatal(err)
	}
	return srpc.NewClientWithMuxedConn(muxed), func() {
		_ = muxed.Close()
	}
}

func pingListenerTestClient(t *testing.T, client srpc.Client) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	if err := client.ExecCall(
		ctx,
		listenerTestServiceID,
		listenerTestPingMethodID,
		new(resource.ResourceRefReleaseRequest),
		new(resource.ResourceRefReleaseResponse),
	); err != nil {
		t.Fatalf("listener test ping: %v", err)
	}
}
func recvListenerTestWatch(t *testing.T, watch srpc.Stream) {
	t.Helper()
	if err := watch.MsgRecv(new(resource.ResourceRefReleaseResponse)); err != nil {
		t.Fatalf("listener test watch: %v", err)
	}
}

func waitListenerClientCount(t *testing.T, status *StatusBroker, want uint32) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		snapshot, waitCh := status.Snapshot()
		if snapshot.ConnectedClients == want {
			return
		}
		select {
		case <-waitCh:
		case <-ctx.Done():
			t.Fatalf("connected clients = %d, want %d", snapshot.ConnectedClients, want)
		}
	}
}
