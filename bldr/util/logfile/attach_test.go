package logfile

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

func TestParseLogFileSpecs(t *testing.T) {
	// Fix the template timestamp for deterministic {ts} paths.
	ts := time.Date(2026, 2, 22, 14, 30, 52, 0, time.UTC)

	// Parse a mixed list and verify the none entries are dropped.
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

	// Verify a list of only none entries produces no specs.
	t.Run("all none", func(t *testing.T) {
		specs, err := ParseLogFileSpecs([]string{"none"}, ts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(specs) != 0 {
			t.Fatalf("expected 0 specs, got %d", len(specs))
		}
	})

	// Verify a bad level in one spec fails the whole parse.
	t.Run("error propagation", func(t *testing.T) {
		_, err := ParseLogFileSpecs([]string{
			"path=./ok.log",
			"level=BOGUS;path=./bad.log",
		}, ts)
		if err == nil {
			t.Error("expected error, got nil")
		}
	})

	// Verify a nil input produces no specs.
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
	// Build a debug logger and a text log file spec.
	dir := t.TempDir()

	// Configure the logger at debug level writing to stderr.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(os.Stderr)

	// Point the spec at the test log path.
	path := filepath.Join(dir, "logs", "test.log")
	specs := []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "text", Path: path},
	}

	// Attach the log files and write one message through the hook.
	cleanup, err := AttachLogFiles(log, specs)
	if err != nil {
		t.Fatalf("AttachLogFiles error: %v", err)
	}
	log.WithField("component", "test").Info("attach test message")
	cleanup()

	// Verify the message reached the log file.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}
	if !strings.Contains(string(data), "attach test message") {
		t.Errorf("log file does not contain expected message, got: %q", string(data))
	}
}

func TestAttachLogFilesJSON(t *testing.T) {
	// Build a debug logger and a JSON log file spec.
	dir := t.TempDir()

	// Configure the logger at debug level writing to stderr.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	log.SetOutput(os.Stderr)

	// Point the spec at the test JSON path.
	path := filepath.Join(dir, "test.json")
	specs := []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "json", Path: path},
	}

	// Attach the log files and write one message through the hook.
	cleanup, err := AttachLogFiles(log, specs)
	if err != nil {
		t.Fatalf("AttachLogFiles error: %v", err)
	}
	log.Info("json attach test")
	cleanup()

	// Verify the message reached the log file in JSON format.
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

// TestAttachLogFilesAppendPreservesExistingRecords pins append semantics for
// the shared log-file path. Two processes starting within the same second
// resolve the same {ts}.log path; the second attach must not destroy records
// the first process already wrote.
func TestAttachLogFilesAppendPreservesExistingRecords(t *testing.T) {
	// Attach the first writer and write ten records to the shared path.
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.log")

	// Build the first logger and attach it to the shared path.
	logA := logrus.New()
	logA.SetLevel(logrus.DebugLevel)
	cleanupA, err := AttachLogFiles(logA, []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "text", Path: path},
	})
	if err != nil {
		t.Fatalf("first AttachLogFiles error: %v", err)
	}
	for i := range 10 {
		logA.Infof("a record %d", i)
	}
	cleanupA()

	// Verify the first writer's records exist on disk.
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after first writer: %v", err)
	}
	if !strings.Contains(string(before), "a record 0") {
		t.Fatalf("first writer records missing before second attach: %q", before)
	}

	// Attach the second writer to the same path and write one record.
	logB := logrus.New()
	logB.SetLevel(logrus.DebugLevel)
	cleanupB, err := AttachLogFiles(logB, []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "text", Path: path},
	})
	if err != nil {
		t.Fatalf("second AttachLogFiles error: %v", err)
	}
	logB.Info("b record 0")
	cleanupB()

	// Verify the second attach preserved every existing record.
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read after second writer: %v", err)
	}
	if bytes.IndexByte(after, 0) >= 0 {
		t.Error("log file contains NUL padding after second attach")
	}
	for i := range 10 {
		if want := fmt.Sprintf("a record %d", i); !strings.Contains(string(after), want) {
			t.Errorf("second attach destroyed existing record %q", want)
		}
	}
	if !strings.Contains(string(after), "b record 0") {
		t.Errorf("second writer record missing: %q", after)
	}
}

// TestAttachLogFilesConcurrentWritersKeepsCompleteRecords verifies that two
// hooks attached to one log file path both keep every record. This models
// overlapping service-startup processes sharing one {ts}.log sink.
func TestAttachLogFilesConcurrentWritersKeepsCompleteRecords(t *testing.T) {
	// Attach two writers to the same log file path.
	dir := t.TempDir()
	path := filepath.Join(dir, "shared.log")

	// Build the shared text log file spec.
	specs := []LogFileSpec{{Level: logrus.DebugLevel, Format: "text", Path: path}}

	// Attach the first writer to the shared path.
	logA := logrus.New()
	logA.SetLevel(logrus.DebugLevel)
	cleanupA, err := AttachLogFiles(logA, specs)
	if err != nil {
		t.Fatalf("first AttachLogFiles error: %v", err)
	}

	// Attach the second writer, releasing the first on failure.
	logB := logrus.New()
	logB.SetLevel(logrus.DebugLevel)
	cleanupB, err := AttachLogFiles(logB, specs)
	if err != nil {
		cleanupA()
		t.Fatalf("second AttachLogFiles error: %v", err)
	}

	// Write 50 records from each writer concurrently and detach both.
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := range 50 {
			logA.Infof("a record %d", i)
		}
	}()
	go func() {
		defer wg.Done()
		for i := range 50 {
			logB.Infof("b record %d", i)
		}
	}()
	wg.Wait()
	cleanupA()
	cleanupB()

	// Verify every record survived without truncation padding.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		t.Error("log file contains NUL padding from truncating writes")
	}
	records := strings.Count(string(data), "time=")
	if records != 100 {
		t.Errorf("record count = %d, want 100 (records were destroyed by a truncating attach)", records)
	}
	for i := range 50 {
		for _, prefix := range []string{"a", "b"} {
			if want := fmt.Sprintf("%s record %d", prefix, i); !strings.Contains(string(data), want) {
				t.Errorf("missing record %q", want)
			}
		}
	}
}

func TestAttachLogFilesCleanup(t *testing.T) {
	// Build a logger and attach one text log file spec.
	dir := t.TempDir()

	// Configure the logger at debug level.
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)

	// Point the spec at the cleanup log path.
	path := filepath.Join(dir, "cleanup.log")
	specs := []LogFileSpec{
		{Level: logrus.DebugLevel, Format: "text", Path: path},
	}

	// Attach the log files and clean up immediately.
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
