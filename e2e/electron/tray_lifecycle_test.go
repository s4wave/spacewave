//go:build !skip_e2e && !js

package electron

import (
	"context"
	"strings"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
)

// TIER: nightly
func TestTrayLifecycleKeepsRuntimeAliveAndActivationRestoresWindow(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	page, err := h.WaitForPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	initialPageCount := len(h.AppPages())
	if err := h.ActivateApp(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := h.WaitForAppPageCount(ctx, initialPageCount); err != nil {
		t.Fatal(err)
	}
	if page.IsClosed() {
		t.Fatal("activation with an open main window should focus it, not close it")
	}

	pages := h.AppPages()
	if len(pages) == 0 {
		t.Fatal("expected at least one app page before close")
	}
	closeAppPages(t, pages)
	if err := h.WaitForNoAppPages(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.waitForCDP(ctx); err != nil {
		t.Fatalf("electron runtime should stay alive after closing windows: %v", err)
	}

	if err := h.ActivateApp(ctx); err != nil {
		t.Fatal(err)
	}
	pages, err = h.WaitForAppPageCount(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if url := pages[0].URL(); !strings.HasPrefix(url, "app://") {
		t.Fatalf("expected activation to restore app:// renderer URL, got %q", url)
	}
}

func closeAppPages(t *testing.T, pages []playwright.Page) {
	t.Helper()

	for _, page := range pages {
		if page.IsClosed() {
			continue
		}
		if err := page.Close(); err != nil {
			t.Fatalf("close app page: %v", err)
		}
	}
}
