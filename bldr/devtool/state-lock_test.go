//go:build !js

package devtool

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	stateLockTestRoleEnv = "SPACEWAVE_DEVTOOL_STATE_LOCK_TEST_ROLE"
	stateLockTestRootEnv = "SPACEWAVE_DEVTOOL_STATE_LOCK_TEST_ROOT"
)

func TestDevtoolStateLockSerializesAndReportsHolder(t *testing.T) {
	if role := os.Getenv(stateLockTestRoleEnv); role != "" {
		runDevtoolStateLockRole(t, role)
		return
	}

	stateRoot := t.TempDir()
	holder := stateLockTestCommand(t, stateRoot, "hold")
	holderIn, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	holderOutPipe, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var holderErr bytes.Buffer
	holder.Stderr = &holderErr
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	holderOut := bufio.NewReader(holderOutPipe)
	ready, err := holderOut.ReadString('\n')
	if err != nil {
		t.Fatalf("holder did not become ready: %v\n%s", err, holderErr.String())
	}
	if strings.TrimSpace(ready) != "ready" {
		t.Fatalf("unexpected holder readiness output: %q", ready)
	}

	waiter := stateLockTestCommand(t, stateRoot, "wait")
	waiterOutPipe, err := waiter.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	waiterErrPipe, err := waiter.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := waiter.Start(); err != nil {
		t.Fatal(err)
	}
	waiterOut := bufio.NewReader(waiterOutPipe)
	waiterErr := bufio.NewReader(waiterErrPipe)
	diagnostic, err := waiterErr.ReadString('\n')
	if err != nil {
		t.Fatalf("waiter did not report the held state lock: %v", err)
	}
	if !strings.Contains(diagnostic, "waiting for pid ") || !strings.Contains(diagnostic, "to release bldr state lock") {
		t.Fatalf("missing collision diagnostic: %q", diagnostic)
	}
	if !strings.Contains(diagnostic, stateRoot) {
		t.Fatalf("diagnostic does not name state root %q: %q", stateRoot, diagnostic)
	}

	if err := holderIn.Close(); err != nil {
		t.Fatal(err)
	}
	acquired, err := waiterOut.ReadString('\n')
	if err != nil {
		t.Fatalf("waiter did not acquire after holder released: %v", err)
	}
	if strings.TrimSpace(acquired) != "acquired" {
		t.Fatalf("unexpected waiter acquire output: %q", acquired)
	}
	if err := waiter.Wait(); err != nil {
		t.Fatalf("waiter failed after acquiring lock: %v", err)
	}
	if err := holder.Wait(); err != nil {
		t.Fatalf("holder failed: %v\n%s", err, holderErr.String())
	}
}

func runDevtoolStateLockRole(t *testing.T, role string) {
	t.Helper()
	stateRoot := os.Getenv(stateLockTestRootEnv)
	if stateRoot == "" {
		t.Fatal("state lock test root is required")
	}
	lock, err := acquireStateLock(context.Background(), nil, stateRoot)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.release()

	switch role {
	case "hold":
		if _, err := os.Stdout.Write([]byte("ready\n")); err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
			t.Fatal(err)
		}
	case "wait":
		if _, err := os.Stdout.Write([]byte("acquired\n")); err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatalf("unknown state lock test role: %s", role)
	}
}

func stateLockTestCommand(t *testing.T, stateRoot, role string) *exec.Cmd {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestDevtoolStateLockSerializesAndReportsHolder$") //nolint:gosec
	cmd.Env = append(os.Environ(),
		stateLockTestRoleEnv+"="+role,
		stateLockTestRootEnv+"="+stateRoot,
	)
	return cmd
}

func TestDevtoolStateLockWaitHonorsContext(t *testing.T) {
	stateRoot := t.TempDir()
	holder := stateLockTestCommand(t, stateRoot, "hold")
	holderIn, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	holderOutPipe, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = holderIn.Close()
		_ = holder.Wait()
	})
	if ready, err := bufio.NewReader(holderOutPipe).ReadString('\n'); err != nil || strings.TrimSpace(ready) != "ready" {
		t.Fatalf("holder readiness = %q, %v", ready, err)
	}

	logReader, logWriter := io.Pipe()
	logger := logrus.New()
	logger.SetOutput(logWriter)
	ctx, cancel := context.WithCancel(context.Background())
	waitDone := make(chan error, 1)
	go func() {
		lock, err := acquireStateLock(ctx, logrus.NewEntry(logger), stateRoot)
		if lock != nil {
			lock.release()
		}
		waitDone <- err
	}()
	if diagnostic, err := bufio.NewReader(logReader).ReadString('\n'); err != nil || !strings.Contains(diagnostic, "waiting") {
		t.Fatalf("wait diagnostic = %q, %v", diagnostic, err)
	}

	cancel()
	select {
	case err := <-waitDone:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("wait returned %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("state-lock wait did not stop promptly after cancellation")
	}

	if err := holderIn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := holder.Wait(); err != nil {
		t.Fatal(err)
	}
	fresh, err := acquireStateLock(context.Background(), nil, stateRoot)
	if err != nil {
		t.Fatalf("fresh waiter did not acquire after cancellation and release: %v", err)
	}
	fresh.release()
}
