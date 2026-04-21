// Package tailwriter provides an io.Writer that forwards all writes to an
// inner writer while keeping the last N complete lines in a ring buffer.
package tailwriter

import (
	"io"
	"strings"
	"sync"
)

// TailWriter wraps a writer, forwarding all writes and keeping the last
// N complete lines in a ring buffer for later retrieval.
type TailWriter struct {
	inner io.Writer
	mu    sync.Mutex
	lines []string
	max   int
	buf   []byte
}

// New creates a TailWriter that keeps the last maxLines lines.
func New(inner io.Writer, maxLines int) *TailWriter {
	return &TailWriter{
		inner: inner,
		max:   maxLines,
	}
}

// Write forwards to the inner writer and captures lines.
func (t *TailWriter) Write(p []byte) (int, error) {
	n, err := t.inner.Write(p)

	t.mu.Lock()
	t.buf = append(t.buf, p[:n]...)
	for {
		idx := strings.IndexByte(string(t.buf), '\n')
		if idx < 0 {
			break
		}
		line := string(t.buf[:idx])
		t.buf = t.buf[idx+1:]
		line = strings.TrimRight(line, "\r")
		if line == "" {
			continue
		}
		t.lines = append(t.lines, line)
		if len(t.lines) > t.max {
			t.lines = t.lines[len(t.lines)-t.max:]
		}
	}
	t.mu.Unlock()

	return n, err
}

// Lines returns the captured tail lines.
func (t *TailWriter) Lines() []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Flush any remaining partial line.
	if len(t.buf) > 0 {
		line := strings.TrimRight(string(t.buf), "\r\n")
		if line != "" {
			t.lines = append(t.lines, line)
			if len(t.lines) > t.max {
				t.lines = t.lines[len(t.lines)-t.max:]
			}
		}
		t.buf = nil
	}

	out := make([]string, len(t.lines))
	copy(out, t.lines)
	return out
}
