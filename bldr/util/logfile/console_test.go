package logfile

import (
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestConsoleHookLevels(t *testing.T) {
	// Create a console hook that subscribes to warning records.
	buf := &safeBuffer{}
	hook := NewConsoleHook(buf, &logrus.TextFormatter{DisableColors: true}, logrus.WarnLevel)

	// Collect the console hook levels for comparison.
	levels := hook.Levels()
	found := make(map[logrus.Level]bool)
	for _, lvl := range levels {
		found[lvl] = true
	}

	// Verify the console hook includes warning levels and excludes verbose levels.
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
	// Create a console hook with a synchronized output buffer.
	buf := &safeBuffer{}
	hook := NewConsoleHook(buf, &logrus.TextFormatter{DisableColors: true}, logrus.InfoLevel)

	// Send a text record through the console hook.
	entry := &logrus.Entry{
		Logger:  logrus.StandardLogger(),
		Level:   logrus.InfoLevel,
		Message: "console test",
		Data:    logrus.Fields{},
	}
	if err := hook.Fire(entry); err != nil {
		t.Fatalf("Fire() error: %v", err)
	}

	// Verify the console hook wrote the record message.
	out := buf.String()
	if !strings.Contains(out, "console test") {
		t.Errorf("expected output to contain 'console test', got %q", out)
	}
}

func TestEnsureLoggerLevelNoOp(t *testing.T) {
	// Create a debug logger and retain its console destination.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	origOut := log.Out

	// Specs at or below logger level should be a no-op.
	specs := []LogFileSpec{
		{Level: logrus.InfoLevel, Format: "text", Path: "/dev/null"},
	}
	EnsureLoggerLevel(log, specs)

	// Verify a less verbose file hook leaves the logger unchanged.
	if log.Out != origOut {
		t.Error("expected logger output to be unchanged")
	}
	if log.GetLevel() != logrus.DebugLevel {
		t.Errorf("expected level DebugLevel, got %v", log.GetLevel())
	}
}

func TestEnsureLoggerLevelRaises(t *testing.T) {
	// Create an info logger with a synchronized console buffer.
	buf := &safeBuffer{}
	log := logrus.New()
	log.SetLevel(logrus.InfoLevel)
	log.SetOutput(buf)
	log.SetFormatter(&logrus.TextFormatter{DisableColors: true})

	// Enable the debug file level on the info logger.
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

	// Verify the console hook excludes debug records.
	log.Debug("debug message")
	if strings.Contains(buf.String(), "debug message") {
		t.Errorf("expected console hook to filter debug message, got %q", buf.String())
	}
}

func TestDiscardConsoleOutputPreservesFileHooks(t *testing.T) {
	for _, test := range []struct {
		name        string
		loggerLevel logrus.Level
		fileLevel   logrus.Level
	}{
		{name: "direct logger output", loggerLevel: logrus.DebugLevel, fileLevel: logrus.DebugLevel},
		{name: "level-filtered console hook", loggerLevel: logrus.InfoLevel, fileLevel: logrus.DebugLevel},
	} {
		t.Run(test.name, func(t *testing.T) {
			// Create the logger and its console and file buffers.
			terminal := &safeBuffer{}
			file := &safeBuffer{}
			log := logrus.New()
			log.SetLevel(test.loggerLevel)
			log.SetOutput(terminal)
			log.SetFormatter(&logrus.TextFormatter{DisableColors: true})

			// Attach the file hook and silence console destinations.
			fileHook := NewFileHook(file, test.fileLevel, "text")
			log.AddHook(fileHook)
			EnsureLoggerLevel(log, []LogFileSpec{{Level: test.fileLevel}})
			DiscardConsoleOutput(log)

			// Deliver a diagnostic record and drain the file hook.
			log.Info("dashboard diagnostic")
			fileHook.Close()

			// Verify the diagnostic reaches only the file destination.
			if output := terminal.String(); output != "" {
				t.Fatalf("terminal output = %q, want empty", output)
			}
			if output := file.String(); !strings.Contains(output, "dashboard diagnostic") {
				t.Fatalf("file output = %q, want diagnostic record", output)
			}
		})
	}
}
