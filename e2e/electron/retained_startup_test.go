//go:build !skip_e2e && !js

package electron

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TIER: nightly
func TestRetainedStateLauncherStartupSmoke(t *testing.T) {
	// Require the shared Electron harness for retained startup checks.
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	// Bound the Electron retained-state relaunch test.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Capture retained-startup diagnostics when an Electron assertion fails.
	diag := &retainedStartupDiagnostics{h: h}
	t.Cleanup(func() {
		if t.Failed() {
			if path, err := diag.writeFailure(); err != nil {
				t.Logf("write retained startup failure diagnostics: %v", err)
			} else {
				t.Logf("retained startup failure diagnostics: %s", path)
			}
		}
	})

	// Record the initial Electron shell URL and runtime log path.
	initialPage, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	initialURL := initialPage.URL()
	diag.initialURL = initialURL
	initialLogPath := h.LastLogFilePath()
	diag.initialLogPath = initialLogPath
	if initialLogPath == "" {
		t.Fatal("expected initial devtool log path")
	}

	// Relaunch Electron and require a new runtime log with the retained shell.
	if err := h.Relaunch(ctx); err != nil {
		t.Fatal(err)
	}
	retainedLogPath := h.LastLogFilePath()
	diag.retainedLogPath = retainedLogPath
	retainedPage, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	retainedURL := retainedPage.URL()
	diag.retainedURL = retainedURL
	if retainedLogPath == "" || retainedLogPath == initialLogPath {
		t.Fatalf("expected distinct retained-start log path, got initial=%q retained=%q", initialLogPath, retainedLogPath)
	}

	// Require the retained Electron log to show cached manifest reuse.
	retainedProofs := []string{
		"keeping cached devtool manifests for startup validation",
		"preflighting startup manifests",
		"reused cached startup manifest build",
	}
	for _, proof := range retainedProofs {
		waitForLogSubstring(t, ctx, retainedLogPath, proof)
	}

	// Save the initial and retained Electron runtime log tails.
	initialTailPath := filepath.Join(h.ArtifactDir(), "retained-startup-initial-log-tail.txt")
	if err := writeLogTail(initialLogPath, initialTailPath); err != nil {
		t.Fatalf("write initial log tail: %v", err)
	}
	retainedTailPath := filepath.Join(h.ArtifactDir(), "retained-startup-retained-log-tail.txt")
	if err := writeLogTail(retainedLogPath, retainedTailPath); err != nil {
		t.Fatalf("write retained log tail: %v", err)
	}

	// Record the retained Electron startup URLs, logs, and reuse evidence.
	breadcrumbPath, err := writeRetainedStartupBreadcrumbs(
		h,
		initialURL,
		retainedURL,
		initialLogPath,
		retainedLogPath,
		initialTailPath,
		retainedTailPath,
		retainedProofs,
	)
	if err != nil {
		t.Fatalf("write smoke breadcrumbs: %v", err)
	}
	t.Logf("retained startup smoke breadcrumbs: %s", breadcrumbPath)
}

type retainedStartupDiagnostics struct {
	h               *Harness
	initialURL      string
	retainedURL     string
	initialLogPath  string
	retainedLogPath string
}

func (d *retainedStartupDiagnostics) writeFailure() (string, error) {
	// Recover missing Electron log paths before writing failure diagnostics.
	if d.initialLogPath == "" {
		d.initialLogPath = d.h.LastLogFilePath()
	}
	if d.retainedLogPath == "" && d.h.LastLogFilePath() != d.initialLogPath {
		d.retainedLogPath = d.h.LastLogFilePath()
	}

	// Save available Electron startup log tails for failure inspection.
	initialTailPath := ""
	if d.initialLogPath != "" {
		initialTailPath = filepath.Join(d.h.ArtifactDir(), "retained-startup-failure-initial-log-tail.txt")
		if err := writeLogTail(d.initialLogPath, initialTailPath); err != nil {
			initialTailPath = "error:" + err.Error()
		}
	}
	retainedTailPath := ""
	if d.retainedLogPath != "" {
		retainedTailPath = filepath.Join(d.h.ArtifactDir(), "retained-startup-failure-retained-log-tail.txt")
		if err := writeLogTail(d.retainedLogPath, retainedTailPath); err != nil {
			retainedTailPath = "error:" + err.Error()
		}
	}

	// Write the retained Electron startup failure diagnostic record.
	path := filepath.Join(d.h.ArtifactDir(), "retained-startup-failure-diagnostics.txt")
	lines := []string{
		"smoke=retained-state-installed-app-launcher-startup",
		"result=failure",
		"initial_url=" + d.initialURL,
		"retained_url=" + d.retainedURL,
		"state_root=" + d.h.StateRoot(),
		"spacewave_data_dir=" + d.h.SpacewaveDataRoot(),
		"initial_log=" + d.initialLogPath,
		"retained_log=" + d.retainedLogPath,
		"initial_log_tail=" + initialTailPath,
		"retained_log_tail=" + retainedTailPath,
		"failure_breadcrumb=retained launch did not complete installed-app startup smoke",
		"",
	}
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func writeRetainedStartupBreadcrumbs(
	h *Harness,
	initialURL string,
	retainedURL string,
	initialLogPath string,
	retainedLogPath string,
	initialTailPath string,
	retainedTailPath string,
	retainedProofs []string,
) (string, error) {
	// Write the retained Electron startup evidence and log references.
	breadcrumbPath := filepath.Join(h.ArtifactDir(), "retained-startup-breadcrumbs.txt")
	lines := []string{
		"smoke=retained-state-installed-app-launcher-startup",
		"initial_url=" + initialURL,
		"retained_url=" + retainedURL,
		"state_root=" + h.StateRoot(),
		"spacewave_data_dir=" + h.SpacewaveDataRoot(),
		"initial_log=" + initialLogPath,
		"retained_log=" + retainedLogPath,
		"initial_log_tail=" + initialTailPath,
		"retained_log_tail=" + retainedTailPath,
		"proof=initial_launcher_shell_ready",
		"proof=retained_launcher_shell_ready",
		"proof=retained_state_root_reused",
	}
	for _, proof := range retainedProofs {
		lines = append(lines, "proof=retained_log:"+proof)
	}
	lines = append(lines,
		"update_breadcrumb=initial launch populated devtool startup manifest state",
		"update_breadcrumb=retained launch kept cached devtool manifests for startup validation",
		"update_breadcrumb=retained launch reused cached startup manifest build during startup preflight",
		"",
	)
	if err := os.WriteFile(breadcrumbPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return "", err
	}
	return breadcrumbPath, nil
}

func writeLogTail(srcPath string, dstPath string) error {
	// Read the Electron runtime log before retaining its bounded tail.
	data, err := os.ReadFile(srcPath)
	if err != nil {
		return err
	}

	// Limit the Electron runtime log tail to the diagnostic byte bound.
	const maxTailBytes = 32 * 1024
	if len(data) > maxTailBytes {
		data = data[len(data)-maxTailBytes:]
	}
	return os.WriteFile(dstPath, data, 0o644)
}

func waitForLogSubstring(t *testing.T, ctx context.Context, path string, want string) {
	// Prepare a bounded wait for the Electron startup log evidence.
	t.Helper()
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	// Observe the Electron runtime log until the requested evidence appears.
	var last string
	for {
		// Read the Electron runtime log and check for the requested evidence.
		data, err := os.ReadFile(path)
		if err == nil {
			last = string(data)
			if strings.Contains(last, want) {
				return
			}
		}

		// Wait for the next log observation or report the last Electron log tail.
		select {
		case <-ctx.Done():
			if len(last) > 4000 {
				last = last[len(last)-4000:]
			}
			t.Fatalf("timeout waiting for log substring %q in %s\nlast log tail:\n%s", want, path, last)
		case <-ticker.C:
		}
	}
}
