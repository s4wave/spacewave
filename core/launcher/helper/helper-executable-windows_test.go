//go:build windows

package launcher_helper

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

func TestPrepareHelperExecutableCopiesOutsideSource(t *testing.T) {
	rootDir := t.TempDir()
	sourceDir := t.TempDir()
	sourcePath := filepath.Join(sourceDir, "spacewave-helper.exe")
	content := []byte("signed helper bytes")
	if err := os.WriteFile(sourcePath, content, 0o600); err != nil {
		t.Fatal(err)
	}

	preparedPath, cleanup, err := prepareHelperExecutable(rootDir, sourcePath)
	if err != nil {
		t.Fatal(err)
	}
	if preparedPath == sourcePath {
		t.Fatal("prepared helper path still points to source")
	}
	if filepath.Dir(preparedPath) != rootDir {
		t.Fatalf("prepared helper dir = %q, want %q", filepath.Dir(preparedPath), rootDir)
	}
	prepared, err := os.ReadFile(preparedPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(prepared, content) {
		t.Fatalf("prepared helper = %q, want %q", prepared, content)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if err := cleanup(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(preparedPath); !os.IsNotExist(err) {
		t.Fatalf("stat cleaned helper: %v", err)
	}
}

func TestPrepareHelperCommandHidesConsole(t *testing.T) {
	cmd := exec.Command("spacewave-helper.exe")
	prepareHelperCommand(cmd)
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if cmd.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW", cmd.SysProcAttr.CreationFlags)
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow is false")
	}
}

func TestLoadingClientStartsPackagedHelper(t *testing.T) {
	helperPath := os.Getenv("SPACEWAVE_HELPER_TEST_PATH")
	if helperPath == "" {
		t.Skip("SPACEWAVE_HELPER_TEST_PATH is not set")
	}
	rootDir := t.TempDir()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	client, err := NewLoadingClient(
		ctx,
		logrus.NewEntry(logrus.New()),
		rootDir,
		helperPath,
		"",
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.SendDismiss(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(rootDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("helper root contains %d entries after close", len(entries))
	}
}
