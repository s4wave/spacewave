//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	s4wave_provider_core "github.com/s4wave/spacewave/core/provider"
	s4wave_sobject_core "github.com/s4wave/spacewave/core/sobject"
	s4wave_space_core "github.com/s4wave/spacewave/core/space"
)

func TestConnectDaemonDoesNotAutostartAfterDialFailure(t *testing.T) {
	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Stub dial and start so autostart is observable.
	var dialCalls int
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialCalls++
		return nil, context.DeadlineExceeded
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		t.Fatal("connectDaemon must not autostart")
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		t.Fatal("unexpected build client call")
		return nil, nil
	}

	// Require a dial failure without starting a daemon.
	_, err := connectDaemon(context.Background(), "/tmp/state")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "no daemon listening") {
		t.Fatalf("unexpected error: %v", err)
	}
	if dialCalls != 1 {
		t.Fatalf("expected 1 dial attempt, got %d", dialCalls)
	}
}

func TestConnectDaemonWithAutostartStartsDaemonAfterDialFailure(t *testing.T) {
	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Count dials and pipe a connection for the second attempt.
	var dialCalls int
	var startStatePath string
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	// Fail the first dial, then start the daemon and return the pipe.
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialCalls++
		if dialCalls == 1 {
			return nil, os.ErrNotExist
		}
		if filepath.Base(sockPath) != socketName {
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

	// Require autostart after the first dial failure.
	client, err := connectDaemonWithAutostart(context.Background(), shortSocketDir(t))
	if err != nil {
		t.Fatalf("connect daemon: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
	if dialCalls != 2 {
		t.Fatalf("expected 2 dial attempts, got %d", dialCalls)
	}
	if startStatePath == "" {
		t.Fatalf("unexpected start state path: %s", startStatePath)
	}
}

func TestConnectDaemonWithAutostartDoesNotAutostartOverExistingSocketAfterTransientDialFailure(t *testing.T) {
	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Create an existing socket file in the state directory.
	statePath := t.TempDir()
	sockPath := filepath.Join(statePath, socketName)
	if err := os.WriteFile(sockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Fail dial and record whether start was called.
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		return nil, context.DeadlineExceeded
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		t.Fatal("must not autostart while an existing daemon socket may still be live")
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		t.Fatal("unexpected build client call")
		return nil, nil
	}

	// Require the existing socket to block autostart.
	_, err := connectDaemonWithAutostart(context.Background(), statePath)
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.Contains(err.Error(), "existing daemon socket") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConnectDaemonWithAutostartLeavesSocketCleanupToLeaseHolder(t *testing.T) {
	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Create an existing socket file in the state directory.
	statePath := t.TempDir()
	sockPath := filepath.Join(statePath, socketName)
	if err := os.WriteFile(sockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	// Count dials and pipe a connection after the socket is removed.
	var dialCalls int
	var startCalled bool
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	// Remove the socket on dial, then start the daemon.
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialCalls++
		if dialCalls == 1 {
			return nil, syscall.ECONNREFUSED
		}
		return connA, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		startCalled = true
		return nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}

	// Require autostart to leave the socket for the lease holder.
	client, err := connectDaemonWithAutostart(context.Background(), statePath)
	if err != nil {
		t.Fatalf("connect daemon: %v", err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
	if !startCalled {
		t.Fatal("expected daemon autostart")
	}
	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("launcher removed the socket without a lease: %v", err)
	}
}

func TestConnectDaemonSkipsAutostartWhenDialSucceeds(t *testing.T) {
	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Pipe a connection and close it when the test ends.
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

	// Return the pipe from dial and record whether start was called.
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

	// Require a successful dial to skip autostart.
	if _, err := connectDaemon(context.Background(), "/tmp/state"); err != nil {
		t.Fatalf("connect daemon: %v", err)
	}
	if startCalled {
		t.Fatal("expected daemon autostart to be skipped")
	}
}

func TestConnectDaemonWithAutostartReturnsAutostartFailure(t *testing.T) {
	// Save the daemon connection hooks and restore them after the test.
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Fail dial and fail the daemon start.
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		return nil, os.ErrNotExist
	}
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		return context.Canceled
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		t.Fatal("unexpected build client call")
		return nil, nil
	}

	// Require the autostart failure to be returned.
	_, err := connectDaemonWithAutostart(context.Background(), shortSocketDir(t))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "start daemon") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSpaceIDFromListResolvesName(t *testing.T) {
	got, err := resolveSpaceIDFromList("Agent clients", []*s4wave_space_core.SpaceSoListEntry{
		testSpaceListEntry("01other", "Other"),
		testSpaceListEntry("01agents", "Agent clients"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "01agents" {
		t.Fatalf("got %q, want 01agents", got)
	}
}

func TestResolveSpaceIDFromListPreservesExactID(t *testing.T) {
	got, err := resolveSpaceIDFromList("01agents", []*s4wave_space_core.SpaceSoListEntry{
		testSpaceListEntry("01agents", "Agent clients"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "01agents" {
		t.Fatalf("got %q, want 01agents", got)
	}
}

func TestResolveSpaceIDFromListKeepsUnknownArgument(t *testing.T) {
	got, err := resolveSpaceIDFromList("missing", []*s4wave_space_core.SpaceSoListEntry{
		testSpaceListEntry("01agents", "Agent clients"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "missing" {
		t.Fatalf("got %q, want missing", got)
	}
}

func testSpaceListEntry(id, name string) *s4wave_space_core.SpaceSoListEntry {
	return &s4wave_space_core.SpaceSoListEntry{
		Entry: &s4wave_sobject_core.SharedObjectListEntry{
			Ref: &s4wave_sobject_core.SharedObjectRef{
				ProviderResourceRef: &s4wave_provider_core.ProviderResourceRef{Id: id},
			},
		},
		SpaceMeta: &s4wave_space_core.SpaceSoMeta{Name: name},
	}
}
