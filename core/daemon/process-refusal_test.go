//go:build !js

package daemon

import (
	"bufio"
	stderrors "errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"github.com/pkg/errors"
)

// TestUnreadyChildTerminationRefused requires both termination errors and handle
// cleanup without waiting for a live child whose termination was refused.
func TestUnreadyChildTerminationRefused(t *testing.T) {
	// Keep the real child under the test's command so it can exit and be reaped
	// after the wrapper releases its separate reference to the same process.
	cmd := exec.Command(os.Args[0], "-test.run=^TestUnreadyChildProcess$", "-test.timeout=30s")
	cmd.Env = append(os.Environ(), "SPACEWAVE_TEST_UNREADY_CHILD=child")
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close() })

	// Capture the child's stdout to observe its readiness line.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdout.Close() })

	// Start the child and join it at cleanup only if it is still running.
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// Only the test's command joins this fixture after custody is released.
		if cmd.ProcessState != nil {
			return
		}
		if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			t.Error(err)
			return
		}
		if err := cmd.Wait(); err != nil {
			if _, ok := stderrors.AsType[*exec.ExitError](err); !ok {
				t.Error(err)
			}
		}
	})
	if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
		t.Fatal(err)
	}

	// Retain a second handle to the child process the test keeps alive.
	retained, err := os.FindProcess(cmd.Process.Pid)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = retained.Release() })

	// Refuse both operations while the child is blocked on input. Detach also
	// fails so release must still reach the process handle and retain all errors.
	treeErr := errors.New("tree termination refused")
	childErr := errors.New("child termination refused")
	detachErr := errors.New("tracking close failed")
	var calls []string

	// Build a process whose termination hooks record calls and fail.
	child := &process{
		cmd: &exec.Cmd{Process: retained},
		kill: func() error {
			calls = append(calls, "tree")
			return treeErr
		},
		killChild: func() error {
			calls = append(calls, "child")
			return childErr
		},
		detach: func() error {
			calls = append(calls, "detach")
			return detachErr
		},
	}
	err = child.stop()

	// Stop must surface every refusal and release the retained handle.
	for _, cause := range []error{treeErr, childErr, detachErr} {
		if !errors.Is(err, cause) {
			t.Fatalf("cleanup lost %v: %v", cause, err)
		}
	}
	if !strings.Contains(err.Error(), "may remain until the OS or child exits") {
		t.Fatalf("cleanup omitted the surviving-process warning: %v", err)
	}
	if got := strings.Join(calls, ","); got != "tree,child,detach" {
		t.Fatalf("cleanup order or attempt count: %s", got)
	}
	if child.cmd.ProcessState != nil {
		t.Fatal("cleanup reaped a child whose termination was refused")
	}

	// The retained handle must be released on every platform.
	if runtime.GOOS == "windows" {
		if err := retained.WithHandle(func(uintptr) {}); err == nil {
			t.Error("cleanup retained the process handle")
		}
	}
	if runtime.GOOS != "windows" && retained.Pid != -1 {
		t.Error("cleanup retained the process identity")
	}

	// The test, not stop, now allows the child to exit and consumes its status.
	if _, err := io.WriteString(stdin, "exit\n"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Wait(); err != nil {
		if _, ok := stderrors.AsType[*exec.ExitError](err); !ok {
			t.Fatal(err)
		}
	}
	if cmd.ProcessState.ExitCode() != 23 {
		t.Fatalf("fixture did not survive refused termination: %v", cmd.ProcessState)
	}
}
