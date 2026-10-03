//go:build !js

package harness

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

const (
	buildLockTestRoleEnv = "SPACEWAVE_HARNESS_BUILD_LOCK_TEST_ROLE"
	buildLockTestDirEnv  = "SPACEWAVE_HARNESS_BUILD_LOCK_TEST_DIR"
)

func TestBuildLockValidatesNameAndCreatesDirectory(t *testing.T) {
	// Verify that build locks reject invalid file names.
	for _, name := range []string{"", ".", "..", "a/b", `a\b`} {
		if lock, err := AcquireBuildLock(context.Background(), nil, t.TempDir(), name); err == nil {
			lock.Release()
			t.Fatalf("AcquireBuildLock accepted %q", name)
		}
	}

	// Acquire a build lock whose directory does not yet exist.
	lockDir := filepath.Join(t.TempDir(), "missing", "lock-dir")
	lock, err := AcquireBuildLock(context.Background(), nil, lockDir, "build")
	if err != nil {
		t.Fatal(err)
	}
	lock.Release()

	// Verify that the released build lock leaves its shared file in place.
	if _, err := os.Stat(filepath.Join(lockDir, "build.lock")); err != nil {
		t.Fatal(err)
	}
}

func TestBuildLockSerializesAndProcessExitReleases(t *testing.T) {
	// Dispatch subprocess holders to their build lock test role.
	if role := os.Getenv(buildLockTestRoleEnv); role != "" {
		runBuildLockTestRole(t, role)
		return
	}

	// Prepare a subprocess that holds the shared build lock.
	lockDir := t.TempDir()
	holder := buildLockTestCommand(t, lockDir, "hold")
	holderIn, err := holder.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	holderOut, err := holder.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Start the holder and require its build lock readiness signal.
	if err := holder.Start(); err != nil {
		t.Fatal(err)
	}
	if ready, err := bufio.NewReader(holderOut).ReadString('\n'); err != nil || strings.TrimSpace(ready) != "ready" {
		t.Fatalf("holder readiness = %q, %v", ready, err)
	}

	// Start a competing build lock acquisition with captured diagnostics.
	logReader, logWriter := io.Pipe()
	logger := logrus.New()
	logger.SetOutput(logWriter)
	acquired := make(chan *BuildLock, 1)
	errs := make(chan error, 1)
	go func() {
		lock, err := AcquireBuildLock(context.Background(), logrus.NewEntry(logger), lockDir, "build")
		if err != nil {
			errs <- err
			return
		}
		acquired <- lock
	}()

	// Verify that the build lock waiter reports the holder PID.
	if diagnostic, err := bufio.NewReader(logReader).ReadString('\n'); err != nil || !strings.Contains(diagnostic, "waiting for pid ") {
		t.Fatalf("wait diagnostic = %q, %v", diagnostic, err)
	}

	// Release the holder and require the waiting acquisition to finish.
	if err := holderIn.Close(); err != nil {
		t.Fatal(err)
	}
	if err := holder.Wait(); err != nil {
		t.Fatal(err)
	}
	select {
	case lock := <-acquired:
		lock.Release()
	case err := <-errs:
		t.Fatal(err)
	}
	_ = logWriter.Close()
	_ = logReader.Close()

	// Start a subprocess that exits while holding the shared build lock.
	exiter := buildLockTestCommand(t, lockDir, "exit")
	exitOut, err := exiter.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := exiter.Start(); err != nil {
		t.Fatal(err)
	}

	// Wait for the lock holder readiness signal and process exit.
	if ready, err := bufio.NewReader(exitOut).ReadString('\n'); err != nil || strings.TrimSpace(ready) != "ready" {
		t.Fatalf("exit holder readiness = %q, %v", ready, err)
	}
	if err := exiter.Wait(); err != nil {
		t.Fatal(err)
	}

	// Verify that process exit makes the build lock available again.
	lock, err := AcquireBuildLock(context.Background(), nil, lockDir, "build")
	if err != nil {
		t.Fatal(err)
	}
	lock.Release()
}

func TestBuildLockReleasesAfterCanceledWait(t *testing.T) {
	// Hold the build lock before starting a cancelable competing acquisition.
	lockDir := t.TempDir()
	holder, err := AcquireBuildLock(context.Background(), nil, lockDir, "build")
	if err != nil {
		t.Fatal(err)
	}

	// Capture diagnostics from a cancelable build lock waiter.
	logReader, logWriter := io.Pipe()
	logger := logrus.New()
	logger.SetOutput(logWriter)
	ctx, cancel := context.WithCancel(context.Background())
	errs := make(chan error, 1)
	go func() {
		_, err := AcquireBuildLock(ctx, logrus.NewEntry(logger), lockDir, "build")
		errs <- err
	}()

	// Wait until the competing build lock acquisition is blocked.
	if _, err := bufio.NewReader(logReader).ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	// Cancel the waiter and release the holder so the kernel wait ends.
	cancel()
	holder.Release()

	// Verify that the canceled waiter returns an error.
	if err := <-errs; err == nil {
		t.Fatal("canceled waiter acquired lock")
	}

	// Close the captured build lock diagnostic pipes.
	_ = logWriter.Close()
	_ = logReader.Close()

	// Verify that a canceled waiter leaves the build lock available.
	lock, err := AcquireBuildLock(context.Background(), nil, lockDir, "build")
	if err != nil {
		t.Fatal(err)
	}
	lock.Release()
}

func runBuildLockTestRole(t *testing.T, role string) {
	// Acquire the shared build lock for this subprocess role.
	t.Helper()
	lock, err := AcquireBuildLock(context.Background(), nil, os.Getenv(buildLockTestDirEnv), "build")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stdout.WriteString("ready\n"); err != nil {
		t.Fatal(err)
	}
	if role == "exit" {
		os.Exit(0)
	}
	defer lock.Release()

	// Hold the build lock until the parent closes the subprocess input.
	if role != "hold" {
		t.Fatalf("unknown role %q", role)
	}
	if _, err := io.Copy(io.Discard, os.Stdin); err != nil {
		t.Fatal(err)
	}
}

func buildLockTestCommand(t *testing.T, lockDir, role string) *exec.Cmd {
	// Prepare a bounded subprocess for the requested build lock test role.
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestBuildLockSerializesAndProcessExitReleases$") //nolint:gosec
	cmd.Env = append(os.Environ(), buildLockTestRoleEnv+"="+role, buildLockTestDirEnv+"="+lockDir)
	return cmd
}
