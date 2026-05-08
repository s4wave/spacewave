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
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	initialPage, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	initialURL := initialPage.URL()
	initialLogPath := h.LastLogFilePath()
	if initialLogPath == "" {
		t.Fatal("expected initial devtool log path")
	}

	if err := h.Relaunch(ctx); err != nil {
		t.Fatal(err)
	}
	retainedPage, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	retainedURL := retainedPage.URL()
	retainedLogPath := h.LastLogFilePath()
	if retainedLogPath == "" || retainedLogPath == initialLogPath {
		t.Fatalf("expected distinct retained-start log path, got initial=%q retained=%q", initialLogPath, retainedLogPath)
	}

	waitForLogSubstring(t, ctx, retainedLogPath, "keeping cached devtool manifests for startup validation")
	waitForLogSubstring(t, ctx, retainedLogPath, "preflighting startup manifests")
	waitForLogSubstring(t, ctx, retainedLogPath, "reused cached startup manifest build")

	breadcrumbPath := filepath.Join(h.ArtifactDir(), "retained-startup-breadcrumbs.txt")
	body := strings.Join([]string{
		"smoke=retained-state-launcher-startup",
		"initial_url=" + initialURL,
		"retained_url=" + retainedURL,
		"state_root=" + h.StateRoot(),
		"spacewave_data_dir=" + h.SpacewaveDataRoot(),
		"initial_log=" + initialLogPath,
		"retained_log=" + retainedLogPath,
		"",
	}, "\n")
	if err := os.WriteFile(breadcrumbPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write smoke breadcrumbs: %v", err)
	}
	t.Logf("retained startup smoke breadcrumbs: %s", breadcrumbPath)
}

func waitForLogSubstring(t *testing.T, ctx context.Context, path string, want string) {
	t.Helper()

	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	var last string
	for {
		data, err := os.ReadFile(path)
		if err == nil {
			last = string(data)
			if strings.Contains(last, want) {
				return
			}
		}

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
