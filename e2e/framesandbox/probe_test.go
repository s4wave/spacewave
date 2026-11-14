//go:build !js

// Package framesandbox probes what a plugin viewer can do inside an opaque
// sandboxed srcdoc iframe embedded in a cross-origin isolated page: which
// loading paths work (blob: modules, an import map of blob: URLs, blob:
// workers, wasm, port transfer) and that the origin's storage and the
// parent's objects stay out of reach.
package framesandbox

import (
	_ "embed"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/fastjson"
	playwright "github.com/mxschmitt/playwright-go"
)

// runEnv gates the probe; it drives real browsers and runs only on the build
// host through its queue.
const runEnv = "RUN_FRAME_SANDBOX_PROBE"

//go:embed probe.html
var probeHTML []byte

// gif1x1 is a transparent one-pixel GIF the other origin serves.
var gif1x1 = []byte{
	0x47, 0x49, 0x46, 0x38, 0x39, 0x61, 0x01, 0x00, 0x01, 0x00, 0x80, 0x00,
	0x00, 0x00, 0x00, 0x00, 0xff, 0xff, 0xff, 0x21, 0xf9, 0x04, 0x01, 0x00,
	0x00, 0x00, 0x00, 0x2c, 0x00, 0x00, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00,
	0x00, 0x02, 0x02, 0x44, 0x01, 0x00, 0x3b,
}

// mustFail names the checks that must not work in the frame: each would let
// a viewer reach the app origin's storage or the parent page.
var mustFail = []string{
	"localStorage",
	"sessionStorage",
	"indexedDB",
	"opfsGetDirectory",
	"cacheStorage",
	"serviceWorkerRegister",
	"parentDocument",
}

// TestFrameSandboxProbe loads the probe page in each browser and logs the
// frame's report. It fails only when a check in mustFail works.
func TestFrameSandboxProbe(t *testing.T) {
	if os.Getenv(runEnv) == "" {
		t.Skip("set " + runEnv + "=1 to drive real browsers")
	}

	// Serve the probe page with the app's cross-origin isolation headers.
	app := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hdr := w.Header()
		hdr.Set("Cross-Origin-Opener-Policy", "same-origin")
		hdr.Set("Cross-Origin-Embedder-Policy", "require-corp")
		hdr.Set("Cross-Origin-Resource-Policy", "same-origin")
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		hdr.Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(probeHTML)
	}))
	defer app.Close()

	// Serve images from a second origin, with or without CORP.
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if corp := r.URL.Query().Get("corp"); corp != "none" {
			w.Header().Set("Cross-Origin-Resource-Policy", corp)
		}
		w.Header().Set("Content-Type", "image/gif")
		_, _ = w.Write(gif1x1)
	}))
	defer other.Close()

	// Use the driver and browsers the build host provides.
	pw, err := playwright.Run(&playwright.RunOptions{SkipInstallBrowsers: true})
	if err != nil {
		t.Fatalf("start playwright: %v", err)
	}
	defer func() { _ = pw.Stop() }()

	pageURL := app.URL + "/?other=" + url.QueryEscape(other.URL)
	browsers := map[string]playwright.BrowserType{
		"chromium": pw.Chromium,
		"firefox":  pw.Firefox,
		"webkit":   pw.WebKit,
	}
	for _, name := range []string{"chromium", "firefox", "webkit"} {
		t.Run(name, func(t *testing.T) {
			report := probeBrowser(t, browsers[name], pageURL)
			keys := make([]string, 0, len(report))
			for k := range report {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				t.Logf("%s %s: %s", name, k, report[k])
			}
			for _, k := range mustFail {
				if v := report[k]; !strings.HasPrefix(v, "error:") {
					t.Errorf("%s: %s must fail in the frame, got %q", name, k, v)
				}
			}
		})
	}
}

// probeBrowser opens pageURL in a fresh headless browser and returns the
// frame's report.
func probeBrowser(t *testing.T, bt playwright.BrowserType, pageURL string) map[string]string {
	browser, err := bt.Launch(playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)})
	if err != nil {
		t.Fatalf("launch: %v", err)
	}
	defer func() { _ = browser.Close() }()

	page, err := browser.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	if _, err := page.Goto(pageURL); err != nil {
		t.Fatalf("goto: %v", err)
	}
	timeout := float64((30 * time.Second).Milliseconds())
	if _, err := page.WaitForFunction("() => window.__probe !== undefined", nil, playwright.PageWaitForFunctionOptions{Timeout: &timeout}); err != nil {
		t.Fatalf("wait for the frame's report: %v", err)
	}
	raw, err := page.Evaluate("() => JSON.stringify(window.__probe)")
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var p fastjson.Parser
	v, err := p.Parse(raw.(string))
	if err != nil {
		t.Fatalf("decode report: %v", err)
	}
	obj, err := v.Object()
	if err != nil {
		t.Fatalf("decode report: %v", err)
	}
	report := map[string]string{}
	obj.Visit(func(k []byte, v *fastjson.Value) {
		report[string(k)] = string(v.GetStringBytes())
	})
	return report
}
