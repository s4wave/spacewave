//go:build !js

package daemon

import (
	"bufio"
	stderrors "errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/pkg/errors"
)

// TestUnreadyChildTerminationError requires tree errors to preserve diagnostics
// while still joining the child and closing platform tracking, in that order.
func TestUnreadyChildTerminationError(t *testing.T) {
	// Cover a live child, signal delivery, and an exit observed before fallback.
	for _, name := range []string{"live", "terminated", "exited"} {
		t.Run(name, func(t *testing.T) {
			// Keep the fixture alive on stdin until cleanup actually terminates it.
			stdout, output, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stdout.Close() })
			t.Cleanup(func() { _ = output.Close() })

			// Launch a fixture process that waits for cleanup on its stdin pipe.
			cmd := exec.Command(os.Args[0], "-test.run=^TestUnreadyChildProcess$", "-test.timeout=30s")
			cmd.Env = append(os.Environ(), "SPACEWAVE_TEST_UNREADY_CHILD=child")
			cmd.Stdout, cmd.Stderr = output, os.Stderr
			stdin, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = stdin.Close() })

			// Prepare and start the child with daemon process tracking enabled.
			if err := prepareDaemonStart(cmd); err != nil {
				t.Fatal(err)
			}
			child, err := startProcess(cmd)
			if err != nil {
				t.Fatal(err)
			}

			// Retain the child operations and observe their cleanup ordering.
			kill, killChild, detach := child.kill, child.killChild, child.detach
			detached := false
			joinedAtDetach := false
			t.Cleanup(func() {
				// Recover a fixture left unreaped by stop without ever signaling
				// a process group after its identity was released.
				if cmd.ProcessState == nil {
					if err := kill(); err != nil {
						t.Error(err)
						return
					}
					if err := cmd.Wait(); err != nil {
						if _, ok := stderrors.AsType[*exec.ExitError](err); !ok {
							t.Error(err)
						}
					}
				}
				if !detached {
					if err := detach(); err != nil {
						t.Error(err)
					}
				}
			})

			// Wait for the child fixture to confirm that it started.
			if err := output.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := bufio.NewReader(stdout).ReadString('\n'); err != nil {
				t.Fatal(err)
			}

			// Fail at the real tree boundary, both before and after delivery.
			// No fallback may retry numeric tree signaling after joining the child.
			treeErr := errors.New("injected tree termination failure")
			child.kill = func() error {
				if cmd.ProcessState != nil {
					return errors.New("tree signaled after its leader was reaped")
				}
				if name != "live" {
					// Observe the exit event without reaping to exercise Windows'
					// already-exited TerminateProcess result deterministically.
					var waitExit func()
					if name == "exited" {
						waitExit = watchChildExit(t, cmd.Process.Pid)
					}
					if err := kill(); err != nil {
						return err
					}
					if waitExit != nil {
						waitExit()
					}
				}
				return treeErr
			}
			childAttempts := 0
			child.killChild = func() error {
				childAttempts++
				if cmd.ProcessState != nil {
					return errors.New("child signaled after it was reaped")
				}
				return killChild()
			}
			child.detach = func() error {
				detached = true
				joinedAtDetach = cmd.ProcessState != nil
				return detach()
			}

			// Bound the regression without using a deadline to discover exit.
			stopped := make(chan error, 1)
			go func() { stopped <- child.stop() }()
			select {
			case err = <-stopped:
			case <-time.After(5 * time.Second):
				// Process.Kill synchronizes with Wait and cannot signal a
				// recycled PID if a broken stop has already reaped the fixture.
				if err := cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
					t.Fatal(err)
				}
				<-stopped
				t.Fatal("failed-start cleanup waited on a live child")
			}
			if !errors.Is(err, treeErr) {
				t.Fatalf("tree termination error was lost: %v", err)
			}
			if childAttempts != 1 {
				t.Errorf("direct-child termination attempts: %d", childAttempts)
			}
			if cmd.ProcessState == nil {
				t.Error("tree termination error abandoned the unreaped child")
			}
			if !detached {
				t.Error("tree termination error leaked platform tracking")
			}
			if detached && !joinedAtDetach {
				t.Error("platform tracking closed before joining child")
			}
		})
	}
}
