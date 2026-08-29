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

	bdb "github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/cli"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	storage_native "github.com/s4wave/spacewave/bldr/storage/native"
	yield_policy "github.com/s4wave/spacewave/core/resource/listener/yieldpolicy"
	"github.com/sirupsen/logrus"
)

const statePathLeaseHolderEnv = "SPACEWAVE_TEST_STATE_PATH_LEASE_HOLDER"

func TestRunServeCommandCompletesTakeoverBeforeBusInitialization(t *testing.T) {
	statePath := shortStatePath(t)
	sockPath := filepath.Join(statePath, socketName)
	shutdownStarted := make(chan struct{})
	finishShutdown := make(chan struct{})
	old := startDesktopLikeListenerWithShutdown(t, t.Context(), sockPath, func() {
		close(shutdownStarted)
		<-finishShutdown
	})

	app := cli.NewApp()
	parentFlags := flag.NewFlagSet("spacewave", flag.ContinueOnError)
	parentFlags.String("state-path", statePath, "state directory path")
	if err := parentFlags.Parse([]string{"--state-path", statePath}); err != nil {
		t.Fatal(err)
	}
	parent := cli.NewContext(app, parentFlags, nil)
	child := cli.NewContext(app, flag.NewFlagSet("serve", flag.ContinueOnError), parent)

	busInitialized := make(chan struct{})
	commandErr := make(chan error, 1)
	go func() {
		commandErr <- runServeCommand(child, func() cli_entrypoint.CliBus {
			close(busInitialized)
			return nil
		}, yield_policy.NewBroker(), "", true, 0)
	}()

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

func TestPrepareDaemonRuntimeRejectsHeldLeaseAfterPeerExit(t *testing.T) {
	statePath := shortStatePath(t)
	holderPID, holderStore := startStatePathLeaseHolder(t, statePath)
	sockPath := filepath.Join(statePath, socketName)

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

	lease, err := prepareDaemonRuntime(t.Context(), logrus.NewEntry(logrus.New()), statePath, sockPath, true)
	if lease != nil {
		_ = lease.release()
		t.Fatal("replacement acquired lease held by another process")
	}
	var heldErr *StatePathLeaseHeldError
	if !errors.As(err, &heldErr) {
		t.Fatalf("expected StatePathLeaseHeldError, got %v", err)
	}
	if heldErr.HolderPID != holderPID {
		t.Fatalf("holder PID = %d, want %d", heldErr.HolderPID, holderPID)
	}
	if heldErr.StorePath != holderStore {
		t.Fatalf("holder store = %q, want %q", heldErr.StorePath, holderStore)
	}
	<-peerExited
	if _, statErr := os.Stat(sockPath); !os.IsNotExist(statErr) {
		t.Fatalf("peer-exit socket remains after takeover: %v", statErr)
	}
}

func TestPrepareDaemonRuntimeCleanHandoffAcquiresLease(t *testing.T) {
	statePath := shortStatePath(t)
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

func TestPrepareDaemonRuntimeRemovesStaleExplicitSocket(t *testing.T) {
	statePath := shortStatePath(t)
	sockPath := filepath.Join(shortStatePath(t), "runtime.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("close stale listener: %v", err)
	}

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

func TestAcquireStatePathLeaseFailsClosedOnUnknownStoreLock(t *testing.T) {
	statePath := shortStatePath(t)
	storePath, err := storage_native.BoltDBPath(statePath, "unknown")
	if err != nil {
		t.Fatalf("resolve provider store: %v", err)
	}
	if err := os.WriteFile(storePath, nil, 0o600); err != nil {
		t.Fatalf("write provider store: %v", err)
	}
	if err := os.WriteFile(storePath+"-lock", make([]byte, bboltReaderTableOffset), 0o600); err != nil {
		t.Fatalf("write provider lock: %v", err)
	}
	if _, err := acquireStatePathLease(statePath); err == nil ||
		!strings.Contains(err.Error(), "unsupported bbolt lock file") {
		t.Fatalf("expected unsupported lock-file error, got %v", err)
	}
}

func TestStatePathLeaseHolderProcess(t *testing.T) {
	statePath := os.Getenv(statePathLeaseHolderEnv)
	if statePath == "" {
		return
	}
	storePath, err := storage_native.BoltDBPath(statePath, "p_test")
	if err != nil {
		t.Fatalf("resolve provider store: %v", err)
	}
	db, err := bdb.Open(storePath, 0o600, &bdb.Options{NoFreelistSync: false})
	if err != nil {
		t.Fatalf("open writable provider store: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Errorf("close writable provider store: %v", err)
		}
	}()
	if _, err := os.Stdout.WriteString(strconv.Itoa(os.Getpid()) + "\t" + storePath + "\n"); err != nil {
		t.Fatalf("report lease holder: %v", err)
	}
	var stop [1]byte
	if _, err := os.Stdin.Read(stop[:]); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("wait for lease release: %v", err)
	}
}

func startStatePathLeaseHolder(t *testing.T, statePath string) (int, string) {
	t.Helper()
	// The subprocess is this test binary; no external input reaches argv.
	cmd := exec.Command(os.Args[0], "-test.run=^TestStatePathLeaseHolderProcess$") //nolint:gosec
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
	t.Cleanup(func() {
		if err := stdin.Close(); err != nil {
			t.Errorf("close holder stdin: %v", err)
		}
		if err := cmd.Wait(); err != nil {
			t.Errorf("wait for holder: %v", err)
		}
	})

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
	return pid, parts[1]
}
