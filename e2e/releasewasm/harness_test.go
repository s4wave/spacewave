//go:build !skip_e2e && !js

package releasewasm

import (
	"fmt"
	"os"
	"slices"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

func TestIgnoreBrowserErrorFiltersGoRuntimeInfoLogs(t *testing.T) {
	for _, msg := range []string{
		`level=debug msg="added directive" directive=LookupBlockStore`,
		`level=info msg="mounted space contents"`,
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

func TestExpectedReleaseWasmHTTPError(t *testing.T) {
	if !isExpectedReleaseWasmHTTPError("http://127.0.0.1:1234/api/auth/config") {
		t.Fatal("expected static auth config probe to be ignored")
	}
	if isExpectedReleaseWasmHTTPError("http://127.0.0.1:1234/b/pa/app.mjs") {
		t.Fatal("expected release asset errors to remain fatal")
	}
}

func TestPersistentBrowserContextLaunchOptionsReuseChromiumOwner(t *testing.T) {
	t.Setenv(chromiumGPUEnv, "1")

	got := persistentBrowserContextLaunchOptions("chromium")
	want := chromiumLaunchOptions(true)
	if got.Headless == nil || want.Headless == nil || *got.Headless != *want.Headless {
		t.Fatalf("persistent headless=%v, want shared Chromium value %v", got.Headless, want.Headless)
	}
	if got.Channel == nil || want.Channel == nil || *got.Channel != *want.Channel {
		t.Fatalf("persistent channel=%v, want shared Chromium value %v", got.Channel, want.Channel)
	}
	if !slices.Equal(got.Args, want.Args) {
		t.Fatalf("persistent args=%v, want shared Chromium args %v", got.Args, want.Args)
	}

	other := persistentBrowserContextLaunchOptions("firefox")
	if other.Headless == nil || !*other.Headless || other.Channel != nil || len(other.Args) != 0 {
		t.Fatalf("non-Chromium persistent options inherited Chromium state: %#v", other)
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
