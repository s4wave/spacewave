//go:build !js

package spacewave_cli

import (
	"context"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/aperturerobotics/starpc/srpc"
)

func TestRunStopRequestsDaemonShutdown(t *testing.T) {
	// Serve the daemon control service and stop the daemon.
	statePath := shortSocketDir(t)
	sockPath := filepath.Join(statePath, socketName)
	shutdownCh := serveDaemonControl(t, sockPath)

	// Confirm the daemon received the shutdown request.
	if err := runStop(t.Context(), sockPath); err != nil {
		t.Fatal(err)
	}
	waitShutdownRequest(t, shutdownCh)
}

// TestStopCommandUsesSocketPathEnv proves stop reaches a daemon serving on
// SPACEWAVE_SOCKET_PATH outside the state path, as serve places it.
func TestStopCommandUsesSocketPathEnv(t *testing.T) {
	// Clear the socket path environment and prepare the state root.
	clearSocketPathEnv(t)

	// Clear the socket path environment and prepare the state root.
	root := shortSocketDir(t)
	sockPath := filepath.Join(root, "s.sock")
	t.Setenv(socketPathEnvVars[0], sockPath)
	shutdownCh := serveDaemonControl(t, sockPath)

	// Run the stop command through the CLI app.
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
	// Prepare the control service listener and mux.
	t.Helper()

	// Accept one connection and serve the control RPCs.
	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })

	// Register the shutdown handler on the mux.
	shutdownCh := make(chan struct{}, 1)
	mux := srpc.NewMux()
	if err := mux.Register(newDaemonControlHandler(func() {
		shutdownCh <- struct{}{}
	})); err != nil {
		t.Fatal(err)
	}
	server := srpc.NewServer(mux)
	go func() {
		// Accept one connection and serve the control RPCs.
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
	statePath, wait := startResettingShutdownPeer(t, true)

	if err := runStop(t.Context(), filepath.Join(statePath, socketName)); err != nil {
		t.Fatalf("stop after peer exit: %v", err)
	}
	wait()
}

func TestRunStopPreservesResetWhileListenerRemains(t *testing.T) {
	statePath, wait := startResettingShutdownPeer(t, false)

	if err := runStop(t.Context(), filepath.Join(statePath, socketName)); err == nil {
		t.Fatal("stop succeeded while the listener remained reachable")
	}
	wait()
}

func startResettingShutdownPeer(t *testing.T, closeListener bool) (string, func()) {
	// Prepare the resetting control peer listener and mux.
	t.Helper()

	// Prepare the resetting control peer listener and mux.
	statePath := shortSocketDir(t)
	lis, err := net.Listen("unix", filepath.Join(statePath, socketName))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = lis.Close() })

	// Register the shutdown handler that resets the connection.
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
		// Accept one connection and serve the control RPCs.
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
	// Replace the daemon autostart hook with a failing stub.
	oldStart := connectDaemonStart
	connectDaemonStart = func(ctx context.Context, statePath string) error {
		t.Fatal("stop should not autostart daemon")
		return nil
	}
	t.Cleanup(func() {
		connectDaemonStart = oldStart
	})

	// Run stop against a state path with no daemon.
	statePath := shortSocketDir(t)

	// Run stop against a state path with no daemon.
	if err := runStop(t.Context(), filepath.Join(statePath, socketName)); err != nil {
		t.Fatal(err)
	}
}
