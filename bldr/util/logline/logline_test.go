package logline

import (
	"io"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/test"
)

// TestWriterLogsLineLevel checks that a plugin's logrus line keeps its level,
// that other lines take the fallback, and that a split line waits for its end.
func TestWriterLogsLineLevel(t *testing.T) {
	// Capture entries from a debug-level logger.
	log, hook := test.NewNullLogger()
	log.SetOutput(io.Discard)
	log.SetLevel(logrus.DebugLevel)
	w := New(logrus.NewEntry(log), logrus.DebugLevel)

	// Write a warning, a bare line, a split error line and an unknown level.
	for _, chunk := range []string{
		"level=warning msg=\"slow World write transaction\" held=12s\n",
		"panic: boom\nlevel=err",
		"or msg=failed\r\n",
		"level=bogus msg=x\n",
	} {
		if n, err := w.Write([]byte(chunk)); err != nil || n != len(chunk) {
			t.Fatalf("write %q: n=%d err=%v", chunk, n, err)
		}
	}

	// Each line logs once, at the level the line specifies or the fallback
	// level.
	want := []struct {
		level logrus.Level
		msg   string
	}{
		{logrus.WarnLevel, "msg=\"slow World write transaction\" held=12s"},
		{logrus.DebugLevel, "panic: boom"},
		{logrus.ErrorLevel, "msg=failed"},
		{logrus.DebugLevel, "level=bogus msg=x"},
	}
	entries := hook.AllEntries()
	if len(entries) != len(want) {
		t.Fatalf("got %d entries, want %d", len(entries), len(want))
	}
	for i, entry := range entries {
		if entry.Level != want[i].level || entry.Message != want[i].msg {
			t.Fatalf("entry %d: got %v %q, want %v %q", i, entry.Level, entry.Message, want[i].level, want[i].msg)
		}
	}
}
