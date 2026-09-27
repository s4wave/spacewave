//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
)

func TestRunStopRequestsDaemonShutdown(t *testing.T) {
	statePath := makeShortDaemonStopStatePath(t, "stop-a")
	t.Cleanup(func() {
		_ = os.RemoveAll(statePath)
	})
	sockPath := filepath.Join(statePath, socketName)
	shutdownCh := serveDaemonControl(t, sockPath)

	if err := runStop(t.Context(), sockPath); err != nil {
		t.Fatal(err)
	}
	waitShutdownRequest(t, shutdownCh)
}

// TestStopCommandUsesSocketPathEnv proves stop reaches a daemon serving on
// SPACEWAVE_SOCKET_PATH outside the state path, as serve places it.
func TestStopCommandUsesSocketPathEnv(t *testing.T) {
	clearSocketPathEnv(t)

	root := makeShortDaemonStopStatePath(t, "stop-env")
	t.Cleanup(func() {
		_ = os.RemoveAll(root)
	})
	sockPath := filepath.Join(root, "s.sock")
	t.Setenv(socketPathEnvVars[0], sockPath)
	shutdownCh := serveDaemonControl(t, sockPath)

	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Commands = []*cli.Command{newStopCommand(nil)}
	args := []string{"spacewave", "stop", "--state-path", filepath.Join(root, "state")}
	if err := app.RunContext(t.Context(), args); err != nil {
		t.Fatal(err)
	}
	waitShutdownRequest(t, shutdownCh)
}

// serveDaemonControl serves the daemon control service for one connection on
// sockPath and reports each shutdown request on the returned channel.
func serveDaemonControl(t *testing.T, sockPath string) <-chan struct{} {
	t.Helper()

	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })

	shutdownCh := make(chan struct{}, 1)
	mux := srpc.NewMux()
	if err := mux.Register(newDaemonControlHandler(func() {
		shutdownCh <- struct{}{}
	})); err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(mux)
	go func() {
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		mp, err := srpc.NewMuxedConn(conn, false, nil)
		if err != nil {
			conn.Close()
			return
		}
		_ = server.AcceptMuxedConn(t.Context(), mp)
	}()
	return shutdownCh
}

func waitShutdownRequest(t *testing.T, shutdownCh <-chan struct{}) {
	t.Helper()

	select {
	case <-shutdownCh:
	case <-time.After(time.Second):
		t.Fatal("expected shutdown request")
	}
}

func TestRunStopConfirmsPeerExitAfterControlStreamReset(t *testing.T) {
	statePath, wait := startResettingShutdownPeer(t, "stop-reset", true)

	if err := runStop(t.Context(), filepath.Join(statePath, socketName)); err != nil {
		t.Fatalf("stop after peer exit: %v", err)
	}
	wait()
}

func TestRunStopPreservesResetWhileListenerRemains(t *testing.T) {
	statePath, wait := startResettingShutdownPeer(t, "stop-live-reset", false)

	if err := runStop(t.Context(), filepath.Join(statePath, socketName)); err == nil {
		t.Fatal("stop succeeded while the listener remained reachable")
	}
	wait()
}

func startResettingShutdownPeer(t *testing.T, name string, closeListener bool) (string, func()) {
	t.Helper()

	statePath := makeShortDaemonStopStatePath(t, name)
	t.Cleanup(func() { _ = os.RemoveAll(statePath) })
	lis, err := net.Listen("unix", filepath.Join(statePath, socketName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })

	var accepted net.Conn
	mux := srpc.NewMux()
	if err := mux.Register(newDaemonControlHandler(func() {
		if closeListener {
			_ = lis.Close()
		}
		_ = accepted.Close()
	})); err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(mux)
	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := lis.Accept()
		if err != nil {
			return
		}
		accepted = conn
		muxed, err := srpc.NewMuxedConn(conn, false, nil)
		if err != nil {
			return
		}
		_ = server.AcceptMuxedConn(t.Context(), muxed)
	}()

	return statePath, func() {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("resetting control peer did not exit")
		}
	}
}

func TestRunStopWithoutDaemonDoesNotAutostart(t *testing.T) {
	oldStart := connectDaemonStart
	connectDaemonStart = func(ctx context.Context, statePath string) (*exec.Cmd, error) {
		t.Fatal("stop should not autostart daemon")
		return nil, nil
	}
	t.Cleanup(func() {
		connectDaemonStart = oldStart
	})

	statePath := makeShortDaemonStopStatePath(t, "stop-b")
	t.Cleanup(func() {
		_ = os.RemoveAll(statePath)
	})

	if err := runStop(t.Context(), filepath.Join(statePath, socketName)); err != nil {
		t.Fatal(err)
	}
}

func makeShortDaemonStopStatePath(t *testing.T, name string) string {
	t.Helper()

	tmpRoot, err := filepath.Abs(".tmp")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tmpRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	statePath := filepath.Join(tmpRoot, name)
	if err := os.RemoveAll(statePath); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(statePath, 0o755); err != nil {
		t.Fatal(err)
	}
	return statePath
}
