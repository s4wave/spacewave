package spacewave_launcher_controller

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// TestRawUpdateInvalidParent rejects values outside the Windows PID domain.
func TestRawUpdateInvalidParent(t *testing.T) {
	for _, raw := range []string{"0", "-1", "4294967296", "invalid"} {
		t.Run(raw, func(t *testing.T) {
			t.Setenv(rawUpdateRelayParentEnv, raw)
			if err := waitRawUpdateRelayParent(); err == nil {
				t.Fatal("accepted an invalid parent identifier")
			}
		})
	}
}

// TestRawUpdateMissingParent accepts a PID no longer present in the process table.
func TestRawUpdateMissingParent(t *testing.T) {
	// Confirm the fixture identifier is absent rather than waiting on a live process.
	const pid = 0xfffffffc
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err == nil {
		if err := windows.CloseHandle(handle); err != nil {
			t.Fatal(err)
		}
		t.Skip("fixture process identifier is in use")
	}
	if !errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		t.Fatal(err)
	}

	// The update no longer needs to wait after the parent has disappeared.
	t.Setenv(rawUpdateRelayParentEnv, strconv.FormatUint(pid, 10))
	if err := waitRawUpdateRelayParent(); err != nil {
		t.Fatalf("missing update parent: %v", err)
	}
}

// TestRawUpdateAlreadyExitedParent accepts a parent that exited before lookup.
func TestRawUpdateAlreadyExitedParent(t *testing.T) {
	// Reap the process before asking the relay to wait for it.
	cmd := exec.CommandContext(t.Context(), "cmd.exe", "/c", "exit", "0")
	if err := cmd.Run(); err != nil {
		t.Fatal(err)
	}
	t.Setenv(rawUpdateRelayParentEnv, strconv.Itoa(cmd.Process.Pid))
	if err := waitRawUpdateRelayParent(); err != nil {
		t.Fatalf("already exited update parent: %v", err)
	}
}

// TestRawUpdateProcessHandoff executes the actual relay and replacement process.
func TestRawUpdateProcessHandoff(t *testing.T) {
	// The copied test executable acts as the installed app and its next version.
	const marker = "spacewave replacement payload"
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if staged := os.Getenv("SPACEWAVE_TEST_STAGED_UPDATE"); staged != "" {
		// Only the replacement contains the appended version marker.
		data, err := os.ReadFile(self)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.HasSuffix(data, []byte(marker)) {
			t.Log("replacement process started")
			return
		}

		// Success exits this process and transfers execution through the relay.
		if err := applyRawBinaryUpdate(self, staged); err != nil {
			t.Fatal(err)
		}
		t.Fatal("update returned without replacing the process")
	}

	// Stage a distinct executable without modifying the running test binary.
	dir := t.TempDir()
	installed := filepath.Join(dir, "installed.exe")
	staged := filepath.Join(dir, "staged.exe")
	if err := copyFileMode(self, installed, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := copyFileMode(self, staged, 0o755); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(staged, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := file.WriteString(marker); err != nil {
		_ = file.Close() // Preserve the write error that prevented staging.
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}

	// Inherited output stays open through both process handoffs.
	cmd := exec.CommandContext(t.Context(), installed, "-test.run=^TestRawUpdateProcessHandoff$", "-test.v")
	cmd.Env = append(os.Environ(), "SPACEWAVE_TEST_STAGED_UPDATE="+staged)
	cmd.WaitDelay = 30 * time.Second
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("update handoff: %v\n%s", err, output)
	}
	if !bytes.Contains(output, []byte("replacement process started")) {
		t.Fatalf("replacement did not report startup:\n%s", output)
	}
	data, err := os.ReadFile(installed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasSuffix(data, []byte(marker)) {
		t.Fatal("installed executable does not contain the replacement payload")
	}
}
