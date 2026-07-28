//go:build !js && !windows

package bldr_tui_host

import (
	"bytes"
	"context"
	"io"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

const (
	hostHelperModeEnv  = "SPACEWAVE_TUI_TEST_HELPER_MODE"
	hostHelperCountEnv = "SPACEWAVE_TUI_TEST_HELPER_COUNT"
)

func TestMain(m *testing.M) {
	if mode := os.Getenv(hostHelperModeEnv); mode != "" {
		os.Exit(runHostTestHelper(mode))
	}
	os.Exit(m.Run())
}

func TestHostReportsReadyBeforeReturnAndStopsOnCancellation(t *testing.T) {
	host := newTestHost(t, "ready-block", 0)
	stdin, input, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	defer input.Close()
	host.config.Stdin = stdin

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- host.Run(ctx, func() { close(ready) })
	}()

	waitSignal(t, ready, "TUI readiness")
	select {
	case err := <-done:
		t.Fatalf("host returned before cancellation: %v", err)
	default:
	}
	cancel()
	if err := waitResult(t, done, "host cancellation"); err != nil {
		t.Fatalf("host cancellation failed: %v", err)
	}
	assertRuntimeClean(t)
}

func TestHostDoesNotTreatNonzeroInterruptExitAsSignal(t *testing.T) {
	host := newTestHost(t, "ready-interrupt-error", 0)
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- host.Run(ctx, func() { close(ready) })
	}()

	waitSignal(t, ready, "TUI readiness")
	cancel()
	err := waitResult(t, done, "host cancellation")
	if err == nil || !strings.Contains(err.Error(), "stop Bun TuiView host") {
		t.Fatalf("expected nonzero interrupt exit, got %v", err)
	}
	assertRuntimeClean(t)
}

func TestHostRejectsExitBeforeReadiness(t *testing.T) {
	host := newTestHost(t, "exit-before-ready", 0)
	err := host.Run(context.Background(), func() {
		t.Fatal("reported readiness after child exited")
	})
	if err == nil || !strings.Contains(err.Error(), "before read") {
		t.Fatalf("expected readiness error, got %v", err)
	}
	assertRuntimeClean(t)
}

func TestHostBoundsCrashRestarts(t *testing.T) {
	countPath := filepath.Join(t.TempDir(), "attempts")
	t.Setenv(hostHelperCountEnv, countPath)
	host := newTestHost(t, "crash", 2)
	readyCalls := 0
	err := host.Run(context.Background(), func() { readyCalls++ })
	if err == nil {
		t.Fatal("expected child failure after bounded restarts")
	}
	if readyCalls != 0 {
		t.Fatalf("reported readiness %d times", readyCalls)
	}
	countData, err := os.ReadFile(countPath)
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.TrimSpace(string(countData)); count != "3" {
		t.Fatalf("expected three attempts, got %q", count)
	}
	assertRuntimeClean(t)
}

func TestRestoreTerminalWritesOnlyToCharacterDevice(t *testing.T) {
	var regular bytes.Buffer
	if err := restoreTerminal(&regular); err != nil {
		t.Fatal(err)
	}
	if regular.Len() != 0 {
		t.Fatalf("wrote terminal restoration to regular writer: %q", regular.String())
	}

	master, terminal, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer terminal.Close()
	if err := restoreTerminal(terminal); err != nil {
		t.Fatal(err)
	}
	got := make([]byte, len(terminalRestore))
	readResult := make(chan error, 1)
	go func() {
		_, err := io.ReadFull(master, got)
		readResult <- err
	}()
	select {
	case err := <-readResult:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out reading terminal restoration")
	}
	if string(got) != terminalRestore {
		t.Fatalf("terminal restoration = %q", string(got))
	}
}

func newTestHost(t *testing.T, mode string, restartLimit uint) *Host {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CACHE_HOME", filepath.Join(home, "cache"))
	t.Setenv(hostHelperModeEnv, mode)
	moduleURL := (&url.URL{
		Scheme: "file",
		Path:   filepath.Join(home, "glados-tui.js"),
	}).String()
	host, err := NewHost(Config{
		BunPath:          os.Args[0],
		ModuleURL:        moduleURL,
		PluginID:         "glados",
		DaemonSocketPath: filepath.Join(home, "daemon.sock"),
		StateStoreID:     "tui/glados",
		RestartLimit:     restartLimit,
		Stdout:           io.Discard,
		Stderr:           io.Discard,
	})
	if err != nil {
		t.Fatal(err)
	}
	return host
}

func assertRuntimeClean(t *testing.T) {
	t.Helper()
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Join(cacheDir, "spacewave", "tui"))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, entry := range entries {
			names = append(names, entry.Name())
		}
		t.Fatalf("TUI runtime entries leaked: %v", names)
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}, operation string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
	}
}

func waitResult(t *testing.T, result <-chan error, operation string) error {
	t.Helper()
	select {
	case err := <-result:
		return err
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", operation)
		return nil
	}
}

func runHostTestHelper(mode string) int {
	ready := func() {
		fd := os.NewFile(3, "TUI readiness")
		if fd == nil {
			return
		}
		_, _ = io.WriteString(fd, readyMarker+"\n")
		_ = fd.Close()
	}

	switch mode {
	case "ready-block":
		ready()
		_, _ = io.Copy(io.Discard, os.Stdin)
		return 0
	case "ready-interrupt-error":
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		ready()
		<-interrupt
		signal.Stop(interrupt)
		return 7
	case "exit-before-ready":
		return 0
	case "crash":
		path := os.Getenv(hostHelperCountEnv)
		count := 0
		if data, err := os.ReadFile(path); err == nil {
			count, _ = strconv.Atoi(strings.TrimSpace(string(data)))
		}
		_ = os.WriteFile(path, []byte(strconv.Itoa(count+1)), 0o600)
		return 9
	default:
		return 11
	}
}
