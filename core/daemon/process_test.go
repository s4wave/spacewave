//go:build !js

package daemon

import (
	"bufio"
	"io"
	"os"
	"os/exec"
	"testing"

	"github.com/pkg/errors"
)

// TestUnreadyChildProcess supplies a command-driven child and optional
// descendant. The inherited stdout stays open until every process exits.
func TestUnreadyChildProcess(t *testing.T) {
	// Ordinary package tests do not enter the subprocess fixture.
	mode := os.Getenv("SPACEWAVE_TEST_UNREADY_CHILD")
	if mode == "" {
		return
	}
	if mode == "descendant" {
		if _, err := os.Stdout.WriteString("descendant\n"); err != nil {
			t.Fatal(err)
		}
		<-t.Context().Done()
		return
	}

	// Descendants inherit this attempt's group or Job and retain their own pipe
	// handle, so EOF in the test proves that termination reached them too.
	if mode == "tree" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestUnreadyChildProcess$", "-test.timeout=30s")
		cmd.Env = append(os.Environ(), "SPACEWAVE_TEST_UNREADY_CHILD=descendant")
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
	}

	// Let the test choose between a live child and an exited, unreaped leader.
	if _, err := os.Stdout.WriteString("child\n"); err != nil {
		t.Fatal(err)
	}
	if _, err := bufio.NewReader(os.Stdin).ReadString('\n'); err != nil {
		t.Fatal(err)
	}
	os.Exit(23)
}

// TestUnreadyChildStopJoins proves failed-start cleanup terminates descendants
// and joins its own child, signaling only while its identity is still retained.
func TestUnreadyChildStopJoins(t *testing.T) {
	for _, mode := range []string{"child", "tree"} {
		for _, exited := range []bool{false, true} {
			name := mode + "/live"
			if exited {
				name = mode + "/exited"
			}
			t.Run(name, func(t *testing.T) {
				// Keep stdout outside exec.Cmd's pipe cleanup: Wait must not
				// manufacture EOF while a surviving descendant holds it open.
				stdout, output, err := os.Pipe()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = stdout.Close() })
				t.Cleanup(func() { _ = output.Close() })
				cmd := exec.Command(os.Args[0], "-test.run=^TestUnreadyChildProcess$", "-test.timeout=30s")
				cmd.Env = append(os.Environ(), "SPACEWAVE_TEST_UNREADY_CHILD="+mode)
				cmd.Stdout, cmd.Stderr = output, os.Stderr
				stdin, err := cmd.StdinPipe()
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() { _ = stdin.Close() })
				if err := prepareDaemonStart(cmd); err != nil {
					t.Fatal(err)
				}
				child, err := startProcess(cmd)
				if err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if cmd.ProcessState == nil {
						if err := child.stop(); err != nil {
							t.Error(err)
						}
					}
				})
				if err := output.Close(); err != nil {
					t.Fatal(err)
				}

				// Observe fixture readiness and subscribe to the OS exit event
				// before allowing the leader to exit without reaping it.
				reader := bufio.NewReader(stdout)
				lines := 1
				if mode == "tree" {
					lines++
				}
				for range lines {
					if _, err := reader.ReadString('\n'); err != nil {
						t.Fatal(err)
					}
				}
				if exited {
					waitExit := watchChildExit(t, cmd.Process.Pid)
					if _, err := io.WriteString(stdin, "exit\n"); err != nil {
						t.Fatal(err)
					}
					waitExit()
				}

				// Assert the signal/reap ordering at the actual tree termination
				// boundary. Cleanup must never signal after Wait has returned.
				kill := child.kill
				child.kill = func() error {
					if cmd.ProcessState != nil {
						return errors.New("tree signaled after its leader was reaped")
					}
					return kill()
				}
				if err := child.stop(); err != nil {
					t.Fatal(err)
				}
				if cmd.ProcessState == nil {
					t.Fatal("unready child was not joined")
				}
				if exited && cmd.ProcessState.ExitCode() != 23 {
					t.Fatalf("fixture had not exited before cleanup: %v", cmd.ProcessState)
				}
				if _, err := reader.ReadByte(); err != io.EOF {
					t.Fatalf("child tree retained stdout after cleanup: %v", err)
				}
			})
		}
	}
}
