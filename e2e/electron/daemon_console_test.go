//go:build !skip_e2e && !js

package electron

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TIER: nightly
func TestDesktopDaemonConsoleKeepsCLIReachableWithoutWindows(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cliBin := buildSpacewaveCLI(t, ctx, h.RepoRoot())
	if _, err := h.WaitForPage(ctx); err != nil {
		t.Fatal(err)
	}

	pages := h.AppPages()
	if len(pages) == 0 {
		t.Fatal("expected at least one app page before close")
	}
	closeAppPages(t, pages)
	if err := h.WaitForNoAppPages(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 4*time.Minute)
		defer restoreCancel()
		if err := h.Relaunch(restoreCtx); err != nil {
			t.Fatalf("restore electron runtime after no-window daemon check: %v", err)
		}
	})
	if err := h.waitForCDP(ctx); err != nil {
		t.Fatalf("electron runtime should stay alive after closing windows: %v", err)
	}

	stdout, stderr, err := runSpacewaveStatus(ctx, cliBin, h.RepoRoot(), h.CLISocketPath())
	if err != nil {
		t.Fatalf("spacewave status failed: %v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	if !strings.Contains(stdout, `"status":"running"`) {
		t.Fatalf("status output missing running state: %s", stdout)
	}
	if !strings.Contains(stdout, h.CLISocketPath()) {
		t.Fatalf("status output missing socket path %q: %s", h.CLISocketPath(), stdout)
	}

	// Native tray menu clicks are covered by Electron-main unit tests. The CDP
	// harness only sees renderer pages, so this e2e proves the backing daemon
	// and CLI contract while every renderer window is closed.
}

func buildSpacewaveCLI(t *testing.T, ctx context.Context, repoRoot string) string {
	t.Helper()

	bin := filepath.Join(t.TempDir(), "spacewave")
	cmd := exec.CommandContext(ctx, "go", "build", "-tags", "skip_e2e", "-o", bin, "./cmd/spacewave")
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build spacewave CLI: %v\n%s", err, out)
	}
	return bin
}

func runSpacewaveStatus(
	ctx context.Context,
	bin string,
	repoRoot string,
	socketPath string,
) (string, string, error) {
	cmd := exec.CommandContext(
		ctx,
		bin,
		"--output",
		"json",
		"status",
		"--socket-path",
		socketPath,
	)
	cmd.Dir = repoRoot
	cmd.Env = os.Environ()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
