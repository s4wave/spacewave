//go:build !js

package wasm

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
)

// DriveReadyResult is the browser-observed evidence that the Drive quickstart
// reached content-ready, beyond merely rendering the file browser frame.
type DriveReadyResult struct {
	Body                      string
	Hash                      string
	ContentReadyMs            int
	QuickstartState           string
	QuickstartProgressReadyMs *int
	QuickstartContentReadyMs  *int
	QuickstartFinishedMs      *int
	QuickstartError           string
}

// WaitForApp waits for the real app runtime, not the prerendered shell, to be
// connected to the Resource SDK.
func WaitForApp(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		const deadline = performance.now() + 120000
		let booted = false
		while (!globalThis.__s4wave_debug?.root) {
			if (!booted && typeof globalThis.__swBoot === 'function') {
				globalThis.__swBoot(window.location.hash || '#/')
				booted = true
			}
			if (performance.now() > deadline) {
				throw new Error('debug context did not initialize before deadline')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return null
	}`)
	if err != nil {
		body, bodyErr := page.Locator("body").TextContent()
		if bodyErr != nil {
			body = "failed to read body text: " + bodyErr.Error()
		}
		t.Fatalf(
			"app not ready: %v\nurl: %s\nbody: %s",
			err,
			page.URL(),
			trimPageText(body),
		)
	}
}

// NavigateHash changes the client-side hash route without reloading the page.
func NavigateHash(t testing.TB, h *Harness, page playwright.Page, hash string) {
	t.Helper()

	_, err := page.Evaluate(h.Script("navigate-hash.ts"), map[string]any{
		"targetHash": hash,
	})
	if err != nil {
		t.Fatalf("navigate hash %q: %v", hash, err)
	}
}

// WaitForDriveShell waits for the drive viewer shell to render.
func WaitForDriveShell(t testing.TB, page playwright.Page) {
	t.Helper()

	err := page.Locator("[data-testid='unixfs-browser']").WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	)
	if err != nil {
		body, bodyErr := page.Locator("body").TextContent()
		if bodyErr != nil {
			body = "failed to read body text: " + bodyErr.Error()
		}
		debug, debugErr := page.Evaluate(`() => JSON.stringify({
			hash: window.location.hash,
			hasDebugRoot: !!globalThis.__s4wave_debug?.root,
			quickstartTiming: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
			bodyHtml: document.body.innerHTML.slice(0, 3000),
			text: document.body.textContent?.slice(0, 1000) ?? '',
			links: Array.from(document.querySelectorAll('link')).map((link) => ({
				href: link.href,
				rel: link.rel,
				loaded: !!link.sheet,
			})),
			testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
				testid: el.getAttribute('data-testid'),
				text: el.textContent?.slice(0, 120) ?? '',
			})),
		})`)
		if debugErr != nil {
			debug = "failed to collect page debug: " + debugErr.Error()
		}
		t.Fatalf(
			"wait for drive viewer: %v\nurl: %s\nbody: %s\ndebug: %v",
			err,
			page.URL(),
			trimPageText(body),
			debug,
		)
	}
}

// EnableQuickstartTimingLogs asks the browser quickstart flow to log each
// phase and publish phase timing for timeout diagnostics.
func EnableQuickstartTimingLogs(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`() => {
		globalThis.__s4waveLogQuickstartTiming = true
	}`)
	if err != nil {
		t.Fatalf("enable quickstart timing logs: %v", err)
	}
}

// WaitForDriveReady waits for the drive viewer to render its demo content.
func WaitForDriveReady(t testing.TB, h *Harness, page playwright.Page) DriveReadyResult {
	t.Helper()

	WaitForDriveShell(t, page)

	raw, err := page.Evaluate(h.Script("wait-for-drive.ts"), map[string]any{
		"deadlineMs": 120000,
	})
	if err != nil {
		t.Fatalf("wait for drive ready: %v", err)
	}
	result := parseDriveReadyResult(t, raw)
	if !strings.Contains(result.Body, "getting-started.md") {
		t.Fatalf("drive ready result did not include getting-started.md: %q", result.Body)
	}
	return result
}

func parseDriveReadyResult(t testing.TB, raw any) DriveReadyResult {
	t.Helper()

	m, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected drive ready result %T: %#v", raw, raw)
	}
	result := DriveReadyResult{
		Body:           stringField(m, "body"),
		Hash:           stringField(m, "hash"),
		ContentReadyMs: intField(m, "contentReadyMs"),
	}
	if timing, ok := m["quickstartTiming"].(map[string]any); ok {
		result.QuickstartState = stringField(timing, "state")
		result.QuickstartProgressReadyMs = optionalIntField(timing, "progressReadyMs")
		result.QuickstartContentReadyMs = optionalIntField(timing, "contentReadyMs")
		result.QuickstartFinishedMs = optionalIntField(timing, "finishedMs")
		result.QuickstartError = stringField(timing, "error")
	}
	return result
}

func stringField(m map[string]any, key string) string {
	v, _ := m[key].(string)
	return v
}

func intField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return 0
	}
}

func optionalIntField(m map[string]any, key string) *int {
	switch v := m[key].(type) {
	case float64:
		n := int(v)
		return &n
	case int:
		return &v
	default:
		return nil
	}
}

func AssertQuickstartContentAfterProgress(t testing.TB, result DriveReadyResult) {
	t.Helper()

	if result.QuickstartError != "" {
		t.Fatalf("quickstart timing recorded an error: %s", result.QuickstartError)
	}
	if result.QuickstartState != "" && result.QuickstartState != "content-ready" {
		t.Fatalf("expected quickstart state content-ready, got %q", result.QuickstartState)
	}
	if result.QuickstartProgressReadyMs == nil {
		t.Fatal("expected quickstart progress-ready timing before Drive content-ready")
	}
	if result.QuickstartContentReadyMs == nil {
		t.Fatal("expected quickstart content-ready timing before Drive content-ready")
	}
	if result.QuickstartFinishedMs == nil {
		t.Fatal("expected quickstart finished timing before Drive content-ready")
	}
	if *result.QuickstartFinishedMs < *result.QuickstartProgressReadyMs {
		t.Fatalf(
			"expected quickstart finished timing after progress-ready, got progress=%s finished=%s",
			formatOptionalMs(result.QuickstartProgressReadyMs),
			formatOptionalMs(result.QuickstartFinishedMs),
		)
	}
	if result.ContentReadyMs < *result.QuickstartProgressReadyMs {
		t.Fatalf(
			"expected Drive content-ready after quickstart progress-ready, got progress=%s content=%dms",
			formatOptionalMs(result.QuickstartProgressReadyMs),
			result.ContentReadyMs,
		)
	}
	if result.ContentReadyMs < *result.QuickstartContentReadyMs {
		t.Fatalf(
			"expected Drive content-ready after quickstart content-ready, got quickstart=%s content=%dms",
			formatOptionalMs(result.QuickstartContentReadyMs),
			result.ContentReadyMs,
		)
	}
}

func formatOptionalMs(v *int) string {
	if v == nil {
		return "<missing>"
	}
	return fmt.Sprintf("%dms", *v)
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

func trimPageText(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) <= 800 {
		return s
	}
	return s[:800] + "..."
}

// parseQuickstartRoute extracts sessionIndex and spaceID from a URL like:
// http://host/#/u/{sessionIndex}/so/{spaceID}/...
func parseQuickstartRoute(rawURL string) (uint32, string, error) {
	hashIdx := strings.Index(rawURL, "#")
	if hashIdx == -1 || hashIdx == len(rawURL)-1 {
		return 0, "", errors.New("missing hash route")
	}

	parts := strings.Split(strings.TrimPrefix(rawURL[hashIdx:], "#"), "/")
	if len(parts) < 5 {
		return 0, "", errors.Errorf("unexpected route %q", rawURL[hashIdx:])
	}
	if parts[1] != "u" || parts[3] != "so" {
		return 0, "", errors.Errorf("unexpected route %q", rawURL[hashIdx:])
	}

	idx, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return 0, "", errors.Wrap(err, "parse session index")
	}
	if parts[4] == "" {
		return 0, "", errors.New("missing space id")
	}

	return uint32(idx), parts[4], nil
}
