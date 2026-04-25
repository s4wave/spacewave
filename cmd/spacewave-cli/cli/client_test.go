//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"strings"
	"testing"
)

func TestConnectDaemonStartsDaemonAfterDialFailure(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	var dialCalls int
	var startStatePath string
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialCalls++
		if dialCalls == 1 {
			return nil, context.DeadlineExceeded
		}
		if want := "/tmp/state/" + socketName; sockPath != want {
			t.Fatalf("unexpected socket path: %s", sockPath)
		}
		return connA, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		startStatePath = statePath
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		if conn != connA {
			t.Fatal("unexpected connection")
		}
		return &sdkClient{conn: conn}, nil
	}

	client, err := connectDaemon(context.Background(), "/tmp/state")
	if err != nil {
		t.Fatalf("connect daemon: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
	if dialCalls != 2 {
		t.Fatalf("expected 2 dial attempts, got %d", dialCalls)
	}
	if startStatePath != "/tmp/state" {
		t.Fatalf("unexpected start state path: %s", startStatePath)
	}
}

func TestConnectDaemonSkipsAutostartWhenDialSucceeds(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	var startCalled bool
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		return connA, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		startCalled = true
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}

	if _, err := connectDaemon(context.Background(), "/tmp/state"); err != nil {
		t.Fatalf("connect daemon: %v", err)
	}
	if startCalled {
		t.Fatal("expected daemon autostart to be skipped")
	}
}

func TestConnectDaemonReturnsAutostartFailure(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		return nil, context.DeadlineExceeded
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		return context.Canceled
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		t.Fatal("unexpected build client call")
		return nil, nil
	}

	_, err := connectDaemon(context.Background(), "/tmp/state")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "start daemon") {
		t.Fatalf("unexpected error: %v", err)
	}
}
