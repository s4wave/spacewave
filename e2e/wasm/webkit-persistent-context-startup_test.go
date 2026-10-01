//go:build !skip_e2e && !js

package wasm

import (
	"net/url"
	"runtime"
	"sync/atomic"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// TestWebKitPersistentContextRuntimeStartup proves the browser plugin runtime
// starts with the durable profile storage required by the WebKit runtime.
func TestWebKitPersistentContextRuntimeStartup(t *testing.T) {
	// Persistent-context startup proof requires macOS WebKit.
	if runtime.GOOS != "darwin" {
		t.Skipf("persistent-context WebKit startup proof requires macOS; os=%s", runtime.GOOS)
	}
	h := harness(t)
	if h.BrowserName() != "webkit" {
		t.Skipf("persistent-context WebKit startup proof; browser=%s", h.BrowserName())
	}

	// Close the shared browser so the persistent context can claim the profile.
	if err := h.browser.Close(); err != nil {
		t.Fatalf("close shared WebKit browser: %v", err)
	}
	h.browser = nil
	t.Cleanup(func() {
		browser, launchErr := h.launchBrowser(h.pw)
		if launchErr != nil {
			t.Errorf("restore shared WebKit browser: %v", launchErr)
			return
		}
		h.browser = browser
	})

	// Launch a persistent WebKit context backed by a durable profile directory.
	browserCtx, err := h.pw.WebKit.LaunchPersistentContext(
		t.TempDir(),
		playwright.BrowserTypeLaunchPersistentContextOptions{
			Headless: new(h.headless),
		},
	)
	if err != nil {
		t.Fatalf("launch persistent WebKit context: %v", err)
	}
	t.Cleanup(func() { _ = browserCtx.Close() })

	// Reuse the context's initial page or open a new one.
	pages := browserCtx.Pages()
	var page playwright.Page
	if len(pages) == 0 {
		page, err = browserCtx.NewPage()
		if err != nil {
			t.Fatalf("new persistent WebKit page: %v", err)
		}
	} else {
		page = pages[0]
	}

	// Count requests the page makes for the browser index document.
	var browserIndexRequests atomic.Int32
	page.OnRequest(func(request playwright.Request) {
		requestURL, parseErr := url.Parse(request.URL())
		if parseErr == nil && requestURL.Path == "/b/__index.html" {
			browserIndexRequests.Add(1)
		}
	})

	// Load the app from the persistent context and wait for startup.
	if _, err := page.Goto(h.BaseURL() + "/#/"); err != nil {
		t.Fatalf("load persistent WebKit app: %v", err)
	}
	WaitForApp(t, page)
	AssertBrowserStartupDone(t, h, page)

	// The persistent context must fetch the browser index exactly once.
	if got := browserIndexRequests.Load(); got != 1 {
		t.Fatalf("browser index requests = %d, want 1", got)
	}
}
