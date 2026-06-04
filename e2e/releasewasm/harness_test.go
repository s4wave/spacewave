//go:build !js

package releasewasm

import (
	"fmt"
	"os"
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

func TestIgnoreBrowserErrorFiltersGoRuntimeInfoLogs(t *testing.T) {
	for _, msg := range []string{
		`level=debug msg="added directive" directive=LookupBlockStore`,
		`level=info msg="quickstart mount space contents return"`,
	} {
		if !ignoreBrowserError(msg) {
			t.Fatalf("expected runtime log to be ignored: %s", msg)
		}
	}

	msg := `level=warning msg="rejecting tx: apply failed" error="space/world/set-settings: operation type was not handled"`
	if ignoreBrowserError(msg) {
		t.Fatalf("expected warning log to remain fatal: %s", msg)
	}
}

func TestBrowserPageErrorMessagePreservesPlaywrightStack(t *testing.T) {
	err := fmt.Errorf("%w: %w", playwright.ErrPlaywright, &playwright.Error{
		Name:    "Error",
		Message: "object is not iterable",
		Stack:   "TypeError: object is not iterable\n    at app.js:1:2",
	})

	msg := browserPageErrorMessage(err)
	if msg != "TypeError: object is not iterable\n    at app.js:1:2" {
		t.Fatalf("unexpected page error message: %q", msg)
	}
}

func TestQuickstartRuntimeTraceDefaultsOffForChromium(t *testing.T) {
	prevHarness := testHarness
	testHarness = &harness{
		artifactDir: t.TempDir(),
		browserName: "chromium",
	}
	t.Cleanup(func() {
		testHarness = prevHarness
	})
	t.Setenv("E2E_RELEASE_WASM_RUNTIME_TRACE", "")

	capture := beginQuickstartRuntimeTrace(t, nil)
	if capture.started {
		t.Fatal("runtime trace started without E2E_RELEASE_WASM_RUNTIME_TRACE=1")
	}
	if capture.info["captured"] != false {
		t.Fatalf("captured = %#v, want false", capture.info["captured"])
	}
	if _, err := os.Stat(capture.info["path"].(string)); !os.IsNotExist(err) {
		t.Fatalf("trace path exists or stat failed: %v", err)
	}
}
