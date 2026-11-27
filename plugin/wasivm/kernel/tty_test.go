package kernel

import (
	"bytes"
	"context"
	"io"
	"sync"
	"testing"
	"time"
)

func TestVirtualTerminalDiscipline(t *testing.T) {
	// Prepare a kernel whose console is a virtual terminal in cooked mode.
	var out bytes.Buffer
	k := &Kernel{termFD: -1}
	k.console = newTTY(k, true, true)
	k.console.stdout = &onlcrWriter{term: k.console, inner: &out}
	in := k.console.in

	// Edit a line with erase and kill, then end it with a carriage return.
	got, eofMarks := in.discipline([]byte("xy\x15ab\x7fc\r"))
	if string(got) != "ac\n" || eofMarks != 0 {
		t.Fatalf("discipline = %q, %d; want %q, 0", got, eofMarks, "ac\n")
	}
	if want := "xy\b \b\b \bab\b \bc\r\n"; out.String() != want {
		t.Fatalf("echo = %q, want %q", out.String(), want)
	}

	// VEOF on an empty line is an end-of-file mark. On a partial line it
	// delivers the line without a newline.
	got, eofMarks = in.discipline([]byte("\x04ab\x04"))
	if string(got) != "ab" || eofMarks != 1 {
		t.Fatalf("discipline = %q, %d; want %q, 1", got, eofMarks, "ab")
	}
}

func TestVirtualTerminalShell(t *testing.T) {
	// Boot the rootfs shell on a virtual terminal fed through a pipe.
	rootfs := rootfsDir(t)
	stdinR, stdinW := io.Pipe()
	var out syncBuffer
	ctx := context.Background()

	// Create the kernel with a virtual terminal console.
	k, err := New(ctx, testRuntime(t, ctx), &Config{
		Stdout:          &out,
		Stderr:          &out,
		Stdin:           stdinR,
		VirtualTerminal: true,
		RootFS:          hostFS(t, rootfs),
		InitPath:        "/bin/sh",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer k.Close(ctx)

	// Boot in the background so the test can type and bound the run.
	runCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- k.Boot(runCtx) }()

	// Type a pipeline with a correction and exit the shell.
	_, _ = stdinW.Write([]byte("echo hellx\x7fo | tr a-z A-Z\rexit\r"))
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Boot: %v", err)
		}
	case <-runCtx.Done():
		t.Fatalf("shell did not exit; console = %q", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("HELLO\r\n")) {
		t.Fatalf("console = %q, want HELLO", out.String())
	}
	if !bytes.Contains(out.Bytes(), []byte("# ")) {
		t.Fatalf("console = %q, want an interactive prompt", out.String())
	}
}

// TestBootStopsOnCancel checks Boot returns when its context is canceled
// while the shell waits for input.
func TestBootStopsOnCancel(t *testing.T) {
	// Start a kernel on a virtual terminal that never receives input.
	rootfs := rootfsDir(t)
	stdinR, _ := io.Pipe()
	prompted := make(chan struct{})
	prompt := &promptWriter{prompted: prompted}
	ctx := context.Background()
	k, err := New(ctx, testRuntime(t, ctx), &Config{
		Stdout:          prompt,
		Stderr:          prompt,
		Stdin:           stdinR,
		VirtualTerminal: true,
		RootFS:          hostFS(t, rootfs),
		InitPath:        "/bin/sh",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer k.Close(ctx)

	// Boot the root's shell.
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- k.Boot(runCtx) }()

	// Cancel once the shell waits at its prompt. Boot must return.
	select {
	case <-prompted:
	case err := <-done:
		t.Fatalf("Boot returned before the prompt: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("shell printed no prompt")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("Boot did not return after cancel")
	}
}

// promptWriter closes prompted once the output contains a root shell prompt.
type promptWriter struct {
	mu       sync.Mutex
	buf      bytes.Buffer
	prompted chan struct{}
}

// Write records output and signals the first prompt.
func (w *promptWriter) Write(p []byte) (int, error) {
	// Record the output.
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf.Write(p)

	// Signal the first prompt.
	if bytes.Contains(w.buf.Bytes(), []byte("# ")) && w.prompted != nil {
		close(w.prompted)
		w.prompted = nil
	}
	return len(p), nil
}

// syncBuffer is a bytes.Buffer safe for concurrent writes and reads.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write appends p to the buffer.
func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// Bytes returns a copy of the buffered bytes.
func (b *syncBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

// String returns the buffered bytes as a string.
func (b *syncBuffer) String() string {
	return string(b.Bytes())
}
