package logfile

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestParseLogFileSpecs(t *testing.T) {
	ts := time.Date(2026, 2, 22, 14, 30, 52, 0, time.UTC)

	t.Run("filters none", func(t *testing.T) {
		specs, err := ParseLogFileSpecs([]string{
			"none",
			"path=./test.log",
			"none",
		}, ts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(specs) != 1 {
			t.Fatalf("expected 1 spec, got %d", len(specs))
		}
		if specs[0].Path != "./test.log" {
			t.Errorf("path = %q, want %q", specs[0].Path, "./test.log")
		}
	})

	t.Run("all none", func(t *testing.T) {
		specs, err := ParseLogFileSpecs([]string{"none"}, ts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(specs) != 0 {
			t.Fatalf("expected 0 specs, got %d", len(specs))
		}
	})

	t.Run("error propagation", func(t *testing.T) {
		_, err := ParseLogFileSpecs([]string{
			"path=./ok.log",
			"level=BOGUS;path=./bad.log",
		}, ts)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	t.Run("empty input", func(t *testing.T) {
		specs, err := ParseLogFileSpecs(nil, ts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(specs) != 0 {
			t.Errorf("expected 0 specs, got %d", len(specs))
		}
	})
}

func TestAttachLogFiles(t *testing.T) {
	dir := t.TempDir()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(os.Stderr)

	path := filepath.Join(dir, "logs", "test.log")
	specs := []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "text", Path: path},
	}

	cleanup, err := AttachLogFiles(log, specs)
	if err != nil {
		t.Fatalf("AttachLogFiles error: %v", err)
	}

	log.WithField("component", "test").Info("attach test message")
	cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "attach test message") {
		t.Errorf("log file does not contain expected message, got: %q", string(data))
	}
}

func TestAttachLogFilesJSON(t *testing.T) {
	dir := t.TempDir()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(os.Stderr)

	path := filepath.Join(dir, "test.json")
	specs := []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "json", Path: path},
	}

	cleanup, err := AttachLogFiles(log, specs)
	if err != nil {
		t.Fatalf("AttachLogFiles error: %v", err)
	}

	log.Info("json attach test")
	cleanup()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "json attach test") {
		t.Errorf("log file does not contain expected message, got: %q", string(data))
	}
	if !strings.Contains(string(data), "{") {
		t.Errorf("expected JSON format, got: %q", string(data))
	}
}

func TestAttachLogFilesCleanup(t *testing.T) {
	dir := t.TempDir()

	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	path := filepath.Join(dir, "cleanup.log")
	specs := []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "text", Path: path},
	}

	cleanup, err := AttachLogFiles(log, specs)
	if err != nil {
		t.Fatalf("AttachLogFiles error: %v", err)
	}

	// Cleanup should not panic even when called immediately.
	cleanup()

	// File should exist.
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected log file to exist after cleanup")
	}
}
