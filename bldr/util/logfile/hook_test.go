package logfile

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/aperturerobotics/fastjson"
	"github.com/sirupsen/logrus"
)

// safeBuffer is a thread-safe bytes.Buffer for concurrent test writes.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

// Write implements io.Writer.
func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

// String returns the buffer contents.
func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestFileHookTextFormat(t *testing.T) {
	buf := &safeBuffer{}
	hook := NewFileHook(buf, logrus.DebugLevel, "text")

	entry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Message: "hello world",
		Data:    logrus.Fields{},
	}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("Fire() error: %v", err)
	}

	hook.Close()

	out := buf.String()
	if !strings.Contains(out, "hello world") {
		t.Errorf("expected output to contain 'hello world', got %q", out)
	}
}

func TestFileHookJSONFormat(t *testing.T) {
	buf := &safeBuffer{}
	hook := NewFileHook(buf, logrus.DebugLevel, "json")

	entry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Message: "json test",
		Data:    logrus.Fields{"key": "value"},
	}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("Fire() error: %v", err)
	}

	hook.Close()

	out := buf.String()
	var p fastjson.Parser
	v, err := p.Parse(strings.TrimSpace(out))
	if err != nil {
		t.Fatalf("output is not valid JSON: %v\noutput: %q", err, out)
	}
	if msg := string(v.GetStringBytes("msg")); msg != "json test" {
		t.Errorf("expected msg 'json test', got %v", msg)
	}
	if key := string(v.GetStringBytes("key")); key != "value" {
		t.Errorf("expected key 'value', got %v", key)
	}
}

func TestFileHookLevelFiltering(t *testing.T) {
	buf := &safeBuffer{}
	hook := NewFileHook(buf, logrus.WarnLevel, "text")

	// Debug entry should not be written (below WARN threshold).
	entry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.DebugLevel,
		Message: "debug message",
		Data:    logrus.Fields{},
	}

	// The hook's Levels() method determines which levels are reported.
	// Check that debug is NOT in the levels list.
	levels := hook.Levels()
	for _, lvl := range levels {
		if lvl == logrus.DebugLevel {
			t.Error("DebugLevel should not be in levels for WARN hook")
		}
	}

	// Verify expected levels are present.
	found := make(map[logrus.Level]bool)
	for _, lvl := range levels {
		found[lvl] = true
	}
	for _, expected := range []logrus.Level{logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel, logrus.WarnLevel} {
		if !found[expected] {
			t.Errorf("expected %v in levels", expected)
		}
	}

	// Fire a warn entry -- should be written.
	warnEntry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.WarnLevel,
		Message: "warn message",
		Data:    logrus.Fields{},
	}
	_ = hook.Fire(warnEntry)

	// Fire a debug entry directly -- it would be written since Fire doesn't
	// filter (logrus does the filtering via Levels()). This verifies Fire works.
	_ = hook.Fire(entry)

	hook.Close()

	out := buf.String()
	if !strings.Contains(out, "warn message") {
		t.Errorf("expected 'warn message' in output, got %q", out)
	}
}

func TestFileHookCloseDrains(t *testing.T) {
	buf := &safeBuffer{}
	hook := NewFileHook(buf, logrus.DebugLevel, "text")

	for range 10 {
		entry := &logrus.Entry{
			Logger:  logrus.StandardLogger(),
			Level:   logrus.InfoLevel,
			Message: "drain test",
			Data:    logrus.Fields{},
		}
		_ = hook.Fire(entry)
	}

	hook.Close()

	out := buf.String()
	count := strings.Count(out, "drain test")
	if count != 10 {
		t.Errorf("expected 10 entries, got %d", count)
	}
}

func TestFileHookConcurrentFire(t *testing.T) {
	buf := &safeBuffer{}
	hook := NewFileHook(buf, logrus.DebugLevel, "text")

	var wg sync.WaitGroup
	for range 50 {
		wg.Go(func() {
			entry := &logrus.Entry{
				Logger:  logrus.StandardLogger(),
				Level:   logrus.InfoLevel,
				Message: "concurrent",
				Data:    logrus.Fields{},
			}
			_ = hook.Fire(entry)
		})
	}
	wg.Wait()
	hook.Close()

	out := buf.String()
	count := strings.Count(out, "concurrent")
	if count != 50 {
		t.Errorf("expected 50 entries, got %d", count)
	}
}
