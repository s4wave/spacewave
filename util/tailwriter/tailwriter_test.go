package tailwriter

import (
	"bytes"
	"testing"
)

func TestBasicCapture(t *testing.T) {
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("line1\nline2\nline3\n"))
	lines := tw.Lines()
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line1" || lines[1] != "line2" || lines[2] != "line3" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestRingBuffer(t *testing.T) {
	var buf bytes.Buffer
	tw := New(&buf, 3)
	tw.Write([]byte("a\nb\nc\nd\ne\n"))
	lines := tw.Lines()
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "c" || lines[1] != "d" || lines[2] != "e" {
		t.Fatalf("expected [c d e], got %v", lines)
	}
}

func TestPartialLine(t *testing.T) {
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("complete\npartial"))
	lines := tw.Lines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "complete" || lines[1] != "partial" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestMultipleWrites(t *testing.T) {
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("hel"))
	tw.Write([]byte("lo\nwor"))
	tw.Write([]byte("ld\n"))
	lines := tw.Lines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "hello" || lines[1] != "world" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestForwardsToInner(t *testing.T) {
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("forwarded\n"))
	if buf.String() != "forwarded\n" {
		t.Fatalf("expected inner to receive data, got %q", buf.String())
	}
}

func TestEmptyLines(t *testing.T) {
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("a\n\n\nb\n"))
	lines := tw.Lines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines (empty skipped), got %d: %v", len(lines), lines)
	}
}

func TestCRLF(t *testing.T) {
	var buf bytes.Buffer
	tw := New(&buf, 5)
	tw.Write([]byte("windows\r\nline\r\n"))
	lines := tw.Lines()
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "windows" || lines[1] != "line" {
		t.Fatalf("unexpected lines: %v", lines)
	}
}

func TestNoWrites(t *testing.T) {
	var buf bytes.Buffer
	tw := New(&buf, 5)
	lines := tw.Lines()
	if len(lines) != 0 {
		t.Fatalf("expected 0 lines, got %d", len(lines))
	}
}
