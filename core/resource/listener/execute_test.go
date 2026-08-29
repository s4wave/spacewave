//go:build !js

package resource_listener

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	resource "github.com/s4wave/spacewave/bldr/resource"
	listener_control "github.com/s4wave/spacewave/core/resource/listener/control"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	"github.com/sirupsen/logrus"
)

const (
	listenerTestServiceID     = "test.listener"
	listenerTestPingMethodID  = "Ping"
	listenerTestWatchMethodID = "Watch"
)

func TestExecuteSkipsMachineSocketInHostedPlugin(t *testing.T) {
	ctx := bldr_plugin.WithPluginContextInfo(
		t.Context(),
		new(bldr_plugin.PluginContextInfo),
	)
	if err := (&Controller{}).Execute(ctx); err != nil {
		t.Fatal(err)
	}
}

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

func TestServeOnceRefusesConnectableSocket(t *testing.T) {
	// Initialize a cancelable listener test context.
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()

	if err := os.MkdirAll(".tmp", 0o755); err != nil {
		t.Fatal(err)
	}

	// Create the temporary socket directory.
	dir, err := os.MkdirTemp(".tmp", "refuse-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	sock := filepath.Join(dir, "spacewave.sock")

	// Bind the connectable Unix listener.
	lis, err := net.ListenUnix("unix", &net.UnixAddr{Name: sock, Net: "unix"})
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()

	_, err = (&Controller{}).serveOnce(
		ctx,
		logrus.NewEntry(logrus.New()),
		srpc.InvokerFunc(func(string, string, srpc.Stream) (bool, error) {
			return false, nil
		}),
		sock,
		yield_policy.NewBroker(),
		NewStatusBroker(),
		true,
		false,
	)
	if err == nil {
		t.Fatal("expected startup refusal")
	}
	if !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("unexpected startup refusal: %v", err)
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
			if err := strm.MsgRecv(new(resource.ResourceClientInitRequest)); err != nil {
				return true, err
			}
			return true, strm.MsgSend(new(resource.ResourceClientInitRequest))
		case listenerTestWatchMethodID:
			if err := strm.MsgRecv(new(resource.ResourceClientInitRequest)); err != nil {
				return true, err
			}
			if err := strm.MsgSend(new(resource.ResourceClientInitRequest)); err != nil {
				return true, err
			}
			select {
			case <-watchNext:
			case <-strm.Context().Done():
				return true, context.Canceled
			}
			return true, strm.MsgSend(new(resource.ResourceClientInitRequest))
		default:
			return false, nil
		}
	}))

	// Start the accept loop with a drainable server.
	serverErr := make(chan error, 1)
	go func() {
		drainClients, err := acceptCountingListener(ctx, listener, srpc.NewServer(mux), status)
		drainClients()
		serverErr <- err
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

	// Connect clients and verify active-stream behavior.
	clientA, closeA := dialListenerTestClient(t, listener.Addr().String())
	defer closeA()
	watchCtx, cancelWatch := context.WithCancel(t.Context())
	defer cancelWatch()
	watchA, err := clientA.NewStream(
		watchCtx,
		listenerTestServiceID,
		listenerTestWatchMethodID,
		new(resource.ResourceClientInitRequest),
	)
	if err != nil {
		t.Fatal(err)
	}
	defer watchA.Close()
	recvListenerTestWatch(t, watchA)

	// Exercise a second client while the first watch remains active.
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

func TestServeOnceReleasesSocketBeforeDrainingConcurrentClients(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	if err := os.MkdirAll(".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	dir, err := os.MkdirTemp(".tmp", "handoff-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	sock := filepath.Join(dir, "d.sock")

	status := NewStatusBroker()
	broker := yield_policy.NewBrokerWithTimeout(5 * time.Second)
	type serveResult struct {
		yielded bool
		err     error
	}
	serveResultCh := make(chan serveResult, 1)
	go func() {
		yielded, err := (&Controller{}).serveOnce(
			ctx,
			logrus.NewEntry(logrus.New()),
			srpc.InvokerFunc(func(string, string, srpc.Stream) (bool, error) {
				return false, nil
			}),
			sock,
			broker,
			status,
			true,
			false,
		)
		serveResultCh <- serveResult{yielded: yielded, err: err}
	}()

	waitListenerListening(t, status)
	watchingClient, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial persistent client: %v", err)
	}
	defer watchingClient.Close()
	waitListenerClientCount(t, status, 1)

	takeoverResult := make(chan error, 1)
	go func() {
		takeoverResult <- listener_control.TakeoverSocket(
			ctx,
			logrus.NewEntry(logrus.New()),
			sock,
		)
	}()
	allowListenerTakeover(t, broker)

	select {
	case err := <-takeoverResult:
		if err != nil {
			t.Fatalf("takeover: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("takeover did not observe socket release")
	}
	select {
	case result := <-serveResultCh:
		if result.err != nil {
			t.Fatalf("serve once: %v", result.err)
		}
		if !result.yielded {
			t.Fatal("serve once did not report a granted yield")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("listener did not drain clients after takeover acknowledgement")
	}
}

func allowListenerTakeover(t *testing.T, broker *yield_policy.Broker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		prompts, waitCh := broker.SnapshotPrompts()
		if len(prompts) != 0 {
			if err := broker.ResolvePrompt(prompts[0].ID, true); err != nil {
				t.Fatalf("allow takeover: %v", err)
			}
			return
		}
		select {
		case <-waitCh:
		case <-ctx.Done():
			t.Fatal("takeover prompt did not arrive")
		}
	}
}

func waitListenerListening(t *testing.T, status *StatusBroker) {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		snapshot, waitCh := status.Snapshot()
		if snapshot.Listening {
			return
		}
		select {
		case <-waitCh:
		case <-ctx.Done():
			t.Fatal("listener did not start")
		}
	}
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
		new(resource.ResourceClientInitRequest),
		new(resource.ResourceClientInitRequest),
	); err != nil {
		t.Fatalf("listener test ping: %v", err)
	}
}

func recvListenerTestWatch(t *testing.T, watch srpc.Stream) {
	t.Helper()
	if err := watch.MsgRecv(new(resource.ResourceClientInitRequest)); err != nil {
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
