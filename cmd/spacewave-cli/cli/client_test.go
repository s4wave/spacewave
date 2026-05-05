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
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

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

	client, err := connectDaemonWithAutostart(context.Background(), "/tmp/state")
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

func TestConnectDaemonWithAutostartDoesNotAutostartOverExistingSocketAfterTransientDialFailure(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	statePath := t.TempDir()
	sockPath := filepath.Join(statePath, socketName)
	if err := os.WriteFile(sockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

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

	_, err := connectDaemonWithAutostart(context.Background(), statePath)
	if err == nil {
		t.Fatal("expected dial error")
	}
	if !strings.Contains(err.Error(), "existing daemon socket") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestConnectDaemonWithAutostartRemovesStaleSocket(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	statePath := t.TempDir()
	sockPath := filepath.Join(statePath, socketName)
	if err := os.WriteFile(sockPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}

	var dialCalls int
	var startCalled bool
	connA, connB := net.Pipe()
	t.Cleanup(func() {
		connA.Close()
		connB.Close()
	})

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
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("expected stale socket removed before autostart, stat err=%v", err)
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

func TestConnectDaemonWithAutostartReturnsAutostartFailure(t *testing.T) {
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

	_, err := connectDaemonWithAutostart(context.Background(), "/tmp/state")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "start daemon") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestResolveSpaceIDFromListResolvesName(t *testing.T) {
	got, err := resolveSpaceIDFromList("Glados", []*s4wave_space_core.SpaceSoListEntry{
		testSpaceListEntry("01other", "Other"),
		testSpaceListEntry("01glados", "Glados"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "01glados" {
		t.Fatalf("got %q, want 01glados", got)
	}
}

func TestResolveSpaceIDFromListPreservesExactID(t *testing.T) {
	got, err := resolveSpaceIDFromList("01glados", []*s4wave_space_core.SpaceSoListEntry{
		testSpaceListEntry("01glados", "Glados"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != "01glados" {
		t.Fatalf("got %q, want 01glados", got)
	}
}

func TestResolveSpaceIDFromListKeepsUnknownArgument(t *testing.T) {
	got, err := resolveSpaceIDFromList("missing", []*s4wave_space_core.SpaceSoListEntry{
		testSpaceListEntry("01glados", "Glados"),
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
