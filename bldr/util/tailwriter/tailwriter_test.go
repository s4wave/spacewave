package tailwriter

import (
	"bytes"
	"testing"
)

func TestBasicCapture(t *testing.T) {
	// Set up TailWriter and provide three complete input lines.
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("line1\nline2\nline3\n"))

	// Read the captured TailWriter lines after the write.
	lines := tw.Lines()

	// Assert TailWriter retains all three input records.
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Assert TailWriter preserves the original line order.
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestRingBuffer(t *testing.T) {
	// Configure a three-line TailWriter and exceed its capacity.
	var buf bytes.Buffer
	tw := New(&buf, 3)
	tw.Write([]byte("a\nb\nc\nd\ne\n"))

	// Read the bounded tail after five lines are written.
	lines := tw.Lines()

	// Assert the ring buffer retains exactly its configured capacity.
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}

	// Assert the tail contains the newest lines in order.
	if lines[0] != "c" || lines[1] != "d" || lines[2] != "e" {
		t.Fatalf("expected [c d e], got %v", lines)
	}
}

func TestPartialLine(t *testing.T) {
	// Set up TailWriter with one complete line and a trailing fragment.
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("complete\npartial"))

	// Read TailWriter's lines, including its trailing fragment.
	lines := tw.Lines()

	// Assert the partial line is returned with the complete line.
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Assert the complete and partial lines retain their order.
	if lines[0] != "complete" || lines[1] != "partial" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestMultipleWrites(t *testing.T) {
	// Configure TailWriter to assemble lines split across writes.
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("hel"))
	tw.Write([]byte("lo\nwor"))
	tw.Write([]byte("ld\n"))

	// Read the lines assembled from the split writes.
	lines := tw.Lines()

	// Assert the split writes produce two complete lines.
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Assert bytes from separate writes retain their line order.
	if lines[0] != "hello" || lines[1] != "world" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestForwardsToInner(t *testing.T) {
	// Set up a buffer to observe bytes forwarded by TailWriter.
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("forwarded\n"))

	// Assert TailWriter forwards the original bytes to its wrapped writer.
	if buf.String() != "forwarded\n" {
		t.Fatalf("expected inner to receive data, got %q", buf.String())
	}
}

func TestEmptyLines(t *testing.T) {
	// Configure TailWriter with both empty and non-empty records.
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("a\n\n\nb\n"))

	// Read the captured lines after writing empty records.
	lines := tw.Lines()

	// Assert empty records do not appear in TailWriter's output.
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (empty skipped), got %d: %v", len(lines), lines)
	}
}

func TestCRLF(t *testing.T) {
	// Set up TailWriter with Windows-style line endings.
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("windows\r\nline\r\n"))

	// Read the captured CRLF records.
	lines := tw.Lines()

	// Assert both CRLF-terminated lines are retained.
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}

	// Assert TailWriter removes carriage returns from each line.
	if lines[0] != "windows" || lines[1] != "line" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestNoWrites(t *testing.T) {
	// Create an empty TailWriter without writing any bytes.
	var buf bytes.Buffer
	tw := New(&buf, 5)

	// Read TailWriter's empty line set.
	lines := tw.Lines()

	// Assert no lines are returned before the first write.
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(lines))
	}
}
