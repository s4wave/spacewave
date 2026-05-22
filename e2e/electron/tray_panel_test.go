//go:build !skip_e2e && !js

package electron

import (
	"context"
	"os"
	"testing"
	"time"
)

// TIER: nightly
func TestTrayPanelScreenshotEvidence(t *testing.T) {
	if os.Getenv("BLDR_ELECTRON_DESKTOP_TRAY_POPOVER") != "1" {
		t.Skip("set BLDR_ELECTRON_DESKTOP_TRAY_POPOVER=1 before harness boot")
	}
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	if err := h.OpenTrayPopover(ctx); err != nil {
		t.Fatal(err)
	}
	path, err := h.CaptureTrayPopoverScreenshot(ctx, "tray-panel-normal.png")
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatalf("empty tray panel screenshot: %s", path)
	}
	t.Logf("tray panel screenshot: %s", path)
}
