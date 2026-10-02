//go:build !js && !wasip1

package spacewave_cli

import (
	"bufio"
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	"github.com/s4wave/spacewave/core/daemon"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	"github.com/sirupsen/logrus"
)

// statePathLeaseHolderEnv selects the isolated runtime lease holder subprocess.
const statePathLeaseHolderEnv = "SPACEWAVE_TEST_STATE_PATH_LEASE_HOLDER"

// TestRunServeCommandCompletesTakeoverBeforeBusInitialization gates writable
// runtime initialization on the previous runtime's completed shutdown.
func TestRunServeCommandCompletesTakeoverBeforeBusInitialization(t *testing.T) {
	// Replace the desktop listener with a shutdown-gated stub.
	statePath := shortSocketDir(t)
	sockPath := filepath.Join(statePath, socketName)
	shutdownStarted := make(chan struct{})
	finishShutdown := make(chan struct{})
	old := startDesktopLikeListenerWithShutdown(t, t.Context(), sockPath, func() {
		close(shutdownStarted)
		<-finishShutdown
	})

	// Build the parent and child CLI contexts.
	app := cli.NewApp()
	parentFlags := flag.NewFlagSet("spacewave", flag.ContinueOnError)
	parentFlags.String("state-path", statePath, "state directory path")
	if err := parentFlags.Parse([]string{"--state-path", statePath}); err != nil {
		t.Fatal(err)
	}
	parent := cli.NewContext(app, parentFlags, nil)
	child := cli.NewContext(app, flag.NewFlagSet("serve", flag.ContinueOnError), parent)

	// Run the serve command and wait for bus initialization.
	busInitialized := make(chan struct{})
	commandErr := make(chan error, 1)
	go func() {
		commandErr <- runServeCommand(child, func() cli_entrypoint.CliBus {
			close(busInitialized)
			return nil
		}, yield_policy.NewBroker(), "", true, 0)
	}()

	// Confirm the serve command waits for the shutdown to finish.
	select {
	case <-shutdownStarted:
	case <-busInitialized:
		t.Fatal("replacement initialized its writable runtime before takeover shutdown completed")
	case <-time.After(5 * time.Second):
		t.Fatal("takeover did not reach the old runtime")
	}
	select {
	case <-busInitialized:
		t.Fatal("replacement initialized its writable runtime while takeover shutdown was pending")
	default:
	}

	// Release the shutdown gate and confirm the command completes.
	close(finishShutdown)
	select {
	case <-busInitialized:
	case <-time.After(5 * time.Second):
		t.Fatal("replacement did not initialize its runtime after takeover")
	}
	if err := <-commandErr; err == nil || !strings.Contains(err.Error(), "bus not initialized") {
		t.Fatalf("serve error = %v, want bus initialization sentinel", err)
	}
	<-old.done
}

// TestPrepareDaemonRuntimeRejectsHeldLeaseAfterPeerExit keeps a live runtime's
// lease exclusive even after its peer disconnects during takeover.
func TestPrepareDaemonRuntimeRejectsHeldLeaseAfterPeerExit(t *testing.T) {
	// Acquire a lease in another process and serve a peer on the socket.
	statePath := shortSocketDir(t)
	_, holderStore := startStatePathLeaseHolder(t, statePath)
	sockPath := filepath.Join(statePath, socketName)

	// Accept and close one peer connection.
	lis, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	peerExited := make(chan struct{})
	go func() {
		defer close(peerExited)
		conn, acceptErr := lis.Accept()
		_ = lis.Close()
		if acceptErr == nil {
			_ = conn.Close()
		}
	}()

	// Attempt to acquire the held lease and expect failure.
	lease, err := prepareDaemonRuntime(t.Context(), logrus.NewEntry(logrus.New()), statePath, sockPath, true)
	if lease != nil {
		_ = lease.release()
		t.Fatal("replacement acquired lease held by another process")
	}
	var heldErr *StatePathLeaseHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("expected StatePathLeaseHeldError, got %v", err)
	}
	if !errors.Is(err, daemon.ErrStarting) {
		t.Fatalf("expected competing runtime startup, got %v", err)
	}
	if heldErr.StorePath != holderStore {
		t.Fatalf("holder store = %q, want %q", heldErr.StorePath, holderStore)
	}
	<-peerExited
	if _, statErr := os.Stat(sockPath); !os.IsNotExist(statErr) {
		t.Fatalf("peer-exit socket remains after takeover: %v", statErr)
	}
}

// TestPrepareDaemonRuntimeCleanHandoffAcquiresLease admits a replacement after
// the previous runtime releases its kernel lease.
func TestPrepareDaemonRuntimeCleanHandoffAcquiresLease(t *testing.T) {
	// Acquire the old runtime lease and serve the old listener.
	statePath := shortSocketDir(t)
	sockPath := filepath.Join(statePath, socketName)
	oldLease, err := acquireStatePathLease(statePath)
	if err != nil {
		t.Fatalf("acquire old runtime lease: %v", err)
	}
	t.Cleanup(func() {
		if err := oldLease.release(); err != nil {
			t.Errorf("release old runtime lease: %v", err)
		}
	})
	old := startDesktopLikeListenerWithShutdown(t, t.Context(), sockPath, func() {
		if err := oldLease.release(); err != nil {
			t.Errorf("release old runtime lease during handoff: %v", err)
		}
	})

	// Hand off the lease to a new runtime and confirm acquisition.
	lease, err := prepareDaemonRuntime(t.Context(), logrus.NewEntry(logrus.New()), statePath, sockPath, true)
	if err != nil {
		t.Fatalf("prepare daemon runtime: %v", err)
	}
	defer func() {
		if err := lease.release(); err != nil {
			t.Errorf("release state path lease: %v", err)
		}
	}()
	<-old.done

	// Confirm the new lease holds the state path after handoff.
	replacement, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen after clean handoff: %v", err)
	}
	if err := replacement.Close(); err != nil {
		t.Fatalf("close replacement listener: %v", err)
	}
	if err := os.Remove(sockPath); err != nil && !os.IsNotExist(err) {
		t.Fatalf("remove replacement socket: %v", err)
	}
}

// TestPrepareDaemonRuntimeRemovesStaleExplicitSocket cleans an explicit socket
// only after acquiring the writable state path lease.
func TestPrepareDaemonRuntimeRemovesStaleExplicitSocket(t *testing.T) {
	// Leave a stale explicit socket on the state path.
	statePath := shortSocketDir(t)
	sockPath := filepath.Join(shortSocketDir(t), "runtime.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close stale listener: %v", err)
	}

	// Prepare the runtime and confirm the stale socket is removed.
	lease, err := prepareDaemonRuntime(
		t.Context(),
		logrus.NewEntry(logrus.New()),
		statePath,
		sockPath,
		false,
	)
	if err != nil {
		t.Fatalf("prepare daemon runtime: %v", err)
	}
	defer lease.release()
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("stale explicit socket remains: %v", err)
	}
}

// TestStatePathLeaseHolderProcess holds the kernel runtime lease until its stdin
// closes; a forced exit deliberately leaves bbolt's persistent metadata behind.
func TestStatePathLeaseHolderProcess(t *testing.T) {
	// Exit early when not running as the lease holder subprocess.
	statePath := os.Getenv(statePathLeaseHolderEnv)
	if statePath == "" {
		return
	}

	// Hold the runtime's kernel lease throughout the subprocess lifetime.
	lease, err := acquireStatePathLease(statePath)
	if err != nil {
		t.Fatalf("acquire runtime lease: %v", err)
	}
	t.Cleanup(func() {
		if err := lease.release(); err != nil {
			t.Errorf("release runtime lease: %v", err)
		}
	})

	// Publish readiness after acquiring the lease, then await its release gate.
	if _, err := os.Stdout.WriteString(strconv.Itoa(os.Getpid()) + "\t" + lease.path + "\n"); err != nil {
		t.Fatalf("report lease holder: %v", err)
	}
	var stop [1]byte
	if _, err := os.Stdin.Read(stop[:]); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("wait for lease release: %v", err)
	}
}

// startStatePathLeaseHolder waits for a subprocess to acquire its runtime lease
// and registers cleanup unless the test has already joined a forced exit.
func startStatePathLeaseHolder(t *testing.T, statePath string) (*exec.Cmd, string) {
	// Build the holder subprocess command and pipe identity output.
	t.Helper()

	// The subprocess is this test binary; no external input reaches argv.
	cmd := exec.Command(os.Args[0], "-test.run=^TestStatePathLeaseHolderProcess$") //nolint:gosec

	// Wire the environment, stdout, and stdin pipes for the holder.
	cmd.Env = append(os.Environ(), statePathLeaseHolderEnv+"="+statePath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("holder stdout: %v", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatalf("holder stdin: %v", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start holder: %v", err)
	}

	// Release the holder's stdin and wait for it on cleanup.
	t.Cleanup(func() {
		// Cmd.Wait already closed the pipes when the test joined a forced exit.
		if cmd.ProcessState != nil {
			return
		}

		// Release the normal holder through stdin before joining its process.
		if err := stdin.Close(); err != nil {
			t.Errorf("close holder stdin: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			t.Errorf("wait for holder: %v", err)
		}
	})

	// Read the holder identity line and parse its PID and store path.
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil {
		t.Fatalf("read holder identity: %v", err)
	}
	parts := strings.SplitN(strings.TrimSpace(line), "\t", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid holder identity %q", line)
	}
	pid, err := strconv.Atoi(parts[0])
	if err != nil {
		t.Fatalf("parse holder PID: %v", err)
	}
	if pid != cmd.Process.Pid {
		t.Fatalf("holder PID = %d, want subprocess PID %d", pid, cmd.Process.Pid)
	}
	return cmd, parts[1]
}

// TestStateLeasePrecedesSocketCleanup rejects another socket under the same
// writable root before it can remove the first runtime's socket pathname.
func TestStateLeasePrecedesSocketCleanup(t *testing.T) {
	// Hold a runtime lease while a second starter sees a stale-looking socket.
	statePath := shortSocketDir(t)
	lease, err := acquireStatePathLease(statePath)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.release()
	socket := filepath.Join(statePath, "other.sock")
	if err := os.WriteFile(socket, []byte("reserved"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Lease rejection must happen before any socket inspection or removal.
	contender, err := prepareDaemonRuntime(t.Context(), nil, statePath, socket, false)
	if contender != nil {
		_ = contender.release()
		t.Fatal("second socket acquired the same writable root")
	}
	if _, ok := errors.AsType[*StatePathLeaseHeldError](err); !ok {
		t.Fatalf("expected lease conflict, got %v", err)
	}
	if value, err := os.ReadFile(socket); err != nil || string(value) != "reserved" {
		t.Fatalf("loser changed socket pathname: %q, %v", value, err)
	}
}
