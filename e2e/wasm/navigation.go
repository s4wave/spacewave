//go:build !js

package wasm

import (
	"strconv"
	"strings"
	"testing"

	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
)

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

// WaitForDriveReady waits for the drive viewer to render its demo content.
func WaitForDriveReady(t testing.TB, h *Harness, page playwright.Page) {
	t.Helper()

	WaitForDriveShell(t, page)

	body, err := page.Locator("[data-testid='unixfs-browser']").TextContent()
	if err == nil && strings.Contains(body, "getting-started.md") {
		return
	}

	_, err = page.Evaluate(h.Script("wait-for-drive.ts"), map[string]any{
		"deadlineMs": 120000,
	})
	if err != nil {
		t.Fatalf("wait for drive ready: %v", err)
	}
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
