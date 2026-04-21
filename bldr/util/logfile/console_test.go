package logfile

import (
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestConsoleHookLevels(t *testing.T) {
	buf := &safeBuffer{}
	hook := NewConsoleHook(buf, &logrus.TextFormatter{DisableColors: true}, logrus.WarnLevel)

	levels := hook.Levels()
	found := make(map[logrus.Level]bool)
	for _, lvl := range levels {
		found[lvl] = true
	}

	for _, expected := range []logrus.Level{logrus.PanicLevel, logrus.FatalLevel, logrus.ErrorLevel, logrus.WarnLevel} {
		if !found[expected] {
			t.Errorf("expected %v in levels", expected)
		}
	}
	if found[logrus.InfoLevel] {
		t.Error("InfoLevel should not be in levels for WARN hook")
	}
	if found[logrus.DebugLevel] {
		t.Error("DebugLevel should not be in levels for WARN hook")
	}
}

func TestConsoleHookFire(t *testing.T) {
	buf := &safeBuffer{}
	hook := NewConsoleHook(buf, &logrus.TextFormatter{DisableColors: true}, logrus.InfoLevel)

	entry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Message: "console test",
		Data:    logrus.Fields{},
	}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("Fire() error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "console test") {
		t.Errorf("expected output to contain 'console test', got %q", out)
	}
}

func TestEnsureLoggerLevelNoOp(t *testing.T) {
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	origOut := log.Out

	// Specs at or below logger level should be a no-op.
	specs := []LogFileSpec{
		{Level: logrus.InfoLevel, Format: "text", Path: "/dev/null"},
	}
	EnsureLoggerLevel(log, specs)

	if log.Out != origOut {
		t.Error("expected logger output to be unchanged")
	}
	if log.GetLevel() != logrus.DebugLevel {
		t.Errorf("expected level DebugLevel, got %v", log.GetLevel())
	}
}

func TestEnsureLoggerLevelRaises(t *testing.T) {
	buf := &safeBuffer{}
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	log.SetOutput(buf)
	log.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	specs := []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "text", Path: "/dev/null"},
	}
	EnsureLoggerLevel(log, specs)

	// Logger level should be raised to DebugLevel.
	if log.GetLevel() != logrus.DebugLevel {
		t.Errorf("expected level DebugLevel, got %v", log.GetLevel())
	}

	// Console output should go through the hook, not Logger.Out directly.
	// Fire an Info entry -- should appear in buf via ConsoleHook.
	log.Info("info message")
	if !strings.Contains(buf.String(), "info message") {
		t.Errorf("expected console hook to write info message, got %q", buf.String())
	}

	// Fire a Debug entry -- should NOT appear in buf (console hook filters at Info).
	buf.mu.Lock()
	buf.buf.Reset()
	buf.mu.Unlock()

	log.Debug("debug message")
	if strings.Contains(buf.String(), "debug message") {
		t.Errorf("expected console hook to filter debug message, got %q", buf.String())
	}
}
