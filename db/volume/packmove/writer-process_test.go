//go:build darwin || linux || windows

package packmove_test

import (
	"bufio"
	"context"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/s4db"
)

// writerFileEnv names the volume file the helper process writes; unset in the
// test binary's own run.
const writerFileEnv = "PACKMOVE_TEST_WRITER_FILE"

// writerPrefix names the blocks the helper process writes.
const writerPrefix = "child"

// TestWriterProcess is the body of the helper process that TestPackSwitch
// starts, standing in for the backend: it holds the file open, as the backend
// does, and adds one World block per round until its input closes. It prints "ready", then "wrote N" after each durable block and
// "done N" on exit.
func TestWriterProcess(t *testing.T) {
	// Run only as the helper process.
	path := os.Getenv(writerFileEnv)
	if path == "" {
		t.Skip("helper process of TestPackSwitch")
	}

	// Stop when the parent closes our input.
	stop := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		close(stop)
	}()

	// Write blocks until told to stop.
	f := openFixture(t, path, s4db.Options{})
	os.Stdout.WriteString("ready\n")
	var written int
	for stopped := false; !stopped; {
		select {
		case <-stop:
			stopped = true
		case <-time.After(2 * time.Millisecond):
			f.put(f.world, writerPrefix+"-"+strconv.Itoa(written))
			written++
			os.Stdout.WriteString("wrote " + strconv.Itoa(written) + "\n")
		}
	}
	f.close()
	os.Stdout.WriteString("done " + strconv.Itoa(written) + "\n")
}

// writerProcess is a running helper process.
type writerProcess struct {
	// cmd is the process.
	cmd *exec.Cmd
	// stdin closes to stop the process.
	stdin io.WriteCloser
	// wrote is the last count the process reported.
	wrote atomic.Int64
	// done receives the final count.
	done chan int
}

// startWriterProcess starts a helper process that writes to the file at path
// and returns once it holds the file.
func startWriterProcess(ctx context.Context, t *testing.T, path string) *writerProcess {
	// Run this test binary as the helper, with pipes for its input and reports.
	t.Helper()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestWriterProcess$", "-test.timeout=120s") //nolint:gosec // re-executes the test binary.
	cmd.Env = append(os.Environ(), writerFileEnv+"="+path)
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	// Start it.
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	// Follow the process's reports.
	w := &writerProcess{cmd: cmd, stdin: stdin, done: make(chan int, 1)}
	ready := make(chan struct{})
	go func() {
		sc := bufio.NewScanner(stdout)
		for sc.Scan() {
			word, count, _ := strings.Cut(sc.Text(), " ")
			n, _ := strconv.Atoi(count)
			switch word {
			case "ready":
				close(ready)
			case "wrote":
				w.wrote.Store(int64(n))
			case "done":
				w.done <- n
			}
		}
	}()
	select {
	case <-ready:
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("writer process did not start")
	}
	t.Cleanup(func() { _ = cmd.Process.Kill() })
	return w
}

// waitWrites waits until the process has written n more blocks.
func (w *writerProcess) waitWrites(t *testing.T, n int64) {
	t.Helper()
	want := w.wrote.Load() + n
	for deadline := time.Now().Add(30 * time.Second); w.wrote.Load() < want; {
		if time.Now().After(deadline) {
			t.Fatal("writer process stopped writing")
		}
		time.Sleep(time.Millisecond)
	}
}

// stop ends the process and returns the number of blocks it wrote.
func (w *writerProcess) stop(t *testing.T) int {
	t.Helper()
	_ = w.stdin.Close()
	select {
	case n := <-w.done:
		if err := w.cmd.Wait(); err != nil {
			t.Fatal(err)
		}
		return n
	case <-time.After(30 * time.Second):
		t.Fatal("writer process did not stop")
		return 0
	}
}

// writerRefs returns the refs of the first n blocks the process writes.
func writerRefs(t *testing.T, n int) []*block.BlockRef {
	t.Helper()
	refs := make([]*block.BlockRef, n)
	for i := range refs {
		ref, err := block.BuildBlockRef(blockData(writerPrefix+"-"+strconv.Itoa(i)), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	return refs
}
