//go:build windows

package launcher_helper

import (
	"context"
	"os"
	"os/exec"
	"testing"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/windows"
)

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
