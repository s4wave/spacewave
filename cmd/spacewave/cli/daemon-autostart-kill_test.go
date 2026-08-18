//go:build !js && (darwin || linux)

package spacewave_cli

import (
	"bufio"
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	resource_server "github.com/s4wave/spacewave/bldr/resource/server"
)

func startFakeDaemonTree(t *testing.T) (*exec.Cmd, int) {
	t.Helper()
	cmd := exec.Command("sh", "-c", `sleep 30 & child=$!; trap 'kill "$child" 2>/dev/null; wait "$child" 2>/dev/null; exit 0' INT; echo "$child"; wait "$child"`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake daemon: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		_ = killDaemonTree(cmd.Process.Pid)
		_, _ = cmd.Process.Wait()
		t.Fatalf("read fake daemon child PID: %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		_ = killDaemonTree(cmd.Process.Pid)
		_, _ = cmd.Process.Wait()
		t.Fatalf("parse fake daemon child PID: %v", err)
	}
	return cmd, childPID
}

func TestTerminateDaemonCmdAllowsGracefulShutdown(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "stopped")
	cmd := exec.Command("sh", "-c", `trap 'printf stopped > "$1"; exit 0' INT; printf 'ready\n'; while :; do sleep 1; done`, "sh", marker)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake daemon: %v", err)
	}
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("fake daemon readiness = %q, %v", line, err)
	}

	if err := terminateDaemonCmd(cmd, ""); err != nil {
		t.Fatalf("terminate daemon: %v", err)
	}
	if got, err := os.ReadFile(marker); err != nil || string(got) != "stopped" {
		t.Fatalf("graceful shutdown marker = %q, %v", got, err)
	}
}

func TestTerminateDaemonCmdKillsDescendantAfterLeaderExit(t *testing.T) {
	cmd := exec.Command("sh", "-c", `sleep 30 </dev/null >/dev/null 2>&1 & printf '%s\n' "$!"`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake daemon: %v", err)
	}
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read descendant PID: %v", err)
	}
	childPID, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatalf("parse descendant PID: %v", err)
	}

	if err := terminateDaemonCmd(cmd, ""); err != nil {
		t.Fatalf("terminate daemon: %v", err)
	}
	if !waitProcessGone(t, childPID) {
		t.Fatalf("daemon descendant PID %d survived leader exit", childPID)
	}
}

func TestTerminateDaemonCmdForcesShutdownAfterGracePeriod(t *testing.T) {
	oldGracePeriod := daemonShutdownGracePeriod
	daemonShutdownGracePeriod = 25 * time.Millisecond
	t.Cleanup(func() { daemonShutdownGracePeriod = oldGracePeriod })

	cmd := exec.Command("sh", "-c", `trap '' INT; printf 'ready\n'; while :; do sleep 1; done`)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start fake daemon: %v", err)
	}
	if line, err := bufio.NewReader(stdout).ReadString('\n'); err != nil || line != "ready\n" {
		t.Fatalf("fake daemon readiness = %q, %v", line, err)
	}

	if err := terminateDaemonCmd(cmd, ""); err != nil {
		t.Fatalf("terminate daemon: %v", err)
	}
	if processAlive(t, cmd.Process.Pid) {
		t.Fatalf("daemon PID %d survived forced shutdown", cmd.Process.Pid)
	}
}

// TestAutostartedDaemonKilledOnClientBuildFailure proves that when the
// daemon is autostarted and the initial Resource client handshake fails,
// the child daemon process group is terminated so no orphan survives.
func TestAutostartedDaemonKilledOnClientBuildFailure(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	// Start a leader and descendant in one process group, matching the
	// production daemon and plugin/helper process relationship.
	daemonCmd, childPID := startFakeDaemonTree(t)
	leaderPid := daemonCmd.Process.Pid
	t.Cleanup(func() { _ = killDaemonTree(leaderPid) })

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})
	dialCount := 0
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialCount++
		if dialCount == 1 {
			return nil, &net.OpError{Op: "dial", Err: context.DeadlineExceeded}
		}
		return clientConn, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) (*exec.Cmd, error) {
		return daemonCmd, nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return nil, &net.OpError{Op: "read", Err: context.DeadlineExceeded}
	}

	_, err := connectDaemonWithAutostart(context.Background(), "/tmp/test-state")
	if err == nil {
		t.Fatal("expected error from connectDaemonWithAutostart")
	}

	// Production cleanup is synchronous: it terminates the whole group and
	// joins the leader before returning. The test must not call Wait itself.
	if !waitProcessGone(t, leaderPid) {
		t.Fatalf("daemon leader PID %d still has a process entry", leaderPid)
	}
	if !waitProcessGone(t, childPID) {
		t.Fatalf("daemon descendant PID %d survived build failure", childPID)
	}
}

// TestAutostartedDaemonSurvivesSuccessfulClientClose proves that when the
// daemon is autostarted and the client handshake succeeds, closing the
// client does not kill the daemon. The process handle is released so the
// daemon persists for subsequent commands.
func TestAutostartedDaemonSurvivesSuccessfulClientClose(t *testing.T) {
	oldDial := connectDaemonDial
	oldBuildClient := connectDaemonBuildClient
	oldStart := connectDaemonStart
	t.Cleanup(func() {
		connectDaemonDial = oldDial
		connectDaemonBuildClient = oldBuildClient
		connectDaemonStart = oldStart
	})

	daemonCmd, childPID := startFakeDaemonTree(t)
	daemonPid := daemonCmd.Process.Pid
	t.Cleanup(func() { _ = killDaemonTree(daemonPid) })

	clientConn, serverConn := net.Pipe()
	t.Cleanup(func() {
		clientConn.Close()
		serverConn.Close()
	})
	dialCount := 0
	connectDaemonDial = func(ctx context.Context, sockPath string) (net.Conn, error) {
		dialCount++
		if dialCount == 1 {
			return nil, &net.OpError{Op: "dial", Err: context.DeadlineExceeded}
		}
		return clientConn, nil
	}
	connectDaemonStart = func(ctx context.Context, statePath string) (*exec.Cmd, error) {
		return daemonCmd, nil
	}
	connectDaemonBuildClient = func(ctx context.Context, conn net.Conn) (*sdkClient, error) {
		return &sdkClient{conn: conn}, nil
	}

	client, err := connectDaemonWithAutostart(context.Background(), "/tmp/test-state")
	if err != nil {
		t.Fatalf("connectDaemonWithAutostart: %v", err)
	}

	// Close the client. The daemon must survive.
	client.close()

	if !processAlive(t, daemonPid) {
		t.Fatalf("daemon PID %d was killed on successful client close", daemonPid)
	}
	if !processAlive(t, childPID) {
		t.Fatalf("daemon descendant PID %d was killed on successful client close", childPID)
	}
}

// TestBuildSDKClientPreservesClientContext proves that the Resource
// client's stream context is not canceled after the Init handshake
// succeeds. buildSDKClient must use a cancel-only context (not a timeout
// context) for NewClient so the stream stays alive after the handshake
// bound expires. clientCancel is stored on the sdkClient and only invoked
// in close().
func TestBuildSDKClientPreservesClientContext(t *testing.T) {
	// Use a Unix socket pair so srpc.NewClientWithConn can open a real
	// muxed connection and resource_client.NewClient can complete Init.
	tmpDir, err := os.MkdirTemp("/tmp", "sw-bld-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(tmpDir) })
	sockPath := filepath.Join(tmpDir, "s.sock")
	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { lis.Close() })

	// Server side: a minimal Resource server that handles Init.
	mux := srpc.NewMux()
	if err := resource_server.NewResourceServer(srpc.NewMux()).Register(mux); err != nil {
		t.Fatal(err)
	}
	srv := srpc.NewServer(mux)
	go func() {
		for {
			conn, err := lis.Accept()
			if err != nil {
				return
			}
			mc, err := srpc.NewMuxedConn(conn, false, nil)
			if err != nil {
				conn.Close()
				continue
			}
			go srv.AcceptMuxedConn(context.Background(), mc)
		}
	}()

	// Client side: dial and build.
	conn, err := net.Dial("unix", sockPath)
	if err != nil {
		t.Fatal(err)
	}

	ctx := t.Context()

	client, err := buildSDKClient(ctx, conn)
	if err != nil {
		conn.Close()
		t.Fatalf("buildSDKClient: %v", err)
	}

	// clientCancel must be set so close can cancel the stream.
	if client.clientCancel == nil {
		t.Fatal("clientCancel is nil after successful build")
	}

	// The context must NOT be canceled yet — the handshake bound must
	// not have killed the stream. Verify by performing a post-Init RPC.
	rpcCtx, rpcCancel := context.WithTimeout(ctx, 5*time.Second)
	defer rpcCancel()
	_, rpcErr := client.root.LookupProvider(rpcCtx, "local")
	// The minimal test Resource server replies with a semantic unimplemented
	// result. Receiving that exact reply proves a complete post-Init RPC
	// traversed the retained client stream after buildSDKClient returned.
	if rpcErr == nil || rpcErr.Error() != "unimplemented" {
		t.Fatalf("post-Init RPC error = %v, want unimplemented", rpcErr)
	}

	// close must cancel the stream context.
	client.close()
}

func waitProcessGone(t *testing.T, pid int) bool {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if !processAlive(t, pid) {
			return true
		}
		select {
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

// processAlive checks whether a process with the given PID is running.
func processAlive(t *testing.T, pid int) bool {
	t.Helper()
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = proc.Signal(syscall.Signal(0))
	return err == nil
}
