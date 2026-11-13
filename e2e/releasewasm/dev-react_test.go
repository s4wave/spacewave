//go:build !skip_e2e && !js

package releasewasm

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"sync"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// TestLocalCDNDevelopmentReact walks the Development React setting through a
// release build: the default boot never asks for react-dev/, turning the
// setting on fills the cache and the reload runs development React, and a
// release file that fails verification leaves production React running.
func TestLocalCDNDevelopmentReact(t *testing.T) {
	// Require the release fixture, whose browser bundle carries react-dev/.
	if os.Getenv(localCDNEnv) != "1" {
		t.Skip("set " + localCDNEnv + "=1 to build and serve the local startup CDN")
	}

	t.Run("on", func(t *testing.T) {
		// Record every request the document and its workers make.
		ctx := testHarness.newBrowserContext(t)
		requests := recordRequests(ctx)
		page, mute := newDevReactPage(t, ctx)
		consoleDev := watchConsole(page, "React DevTools")

		// The default boot reaches a session without touching react-dev/.
		openLocalSession(t, page)
		boot := requests.take()
		writeRequestList(t, "default-boot", boot)
		t.Logf("default boot: %d requests, %d under react-dev/", len(boot), countReactDev(boot))
		if n := countReactDev(boot); n != 0 {
			t.Fatalf("default boot made %d react-dev/ requests", n)
		}
		requireProductionReact(t, page)

		// Turning the setting on fills the cache, then offers the reload.
		openDeveloperSettings(t, page)
		watchReactDevStatus(t, page)
		clickButton(t, page, "Turn on")
		waitButton(t, page, "Reload")
		states := readReactDevStatus(t, page)
		t.Logf("states shown: %q", states)
		if !slices.ContainsFunc(states, func(s string) bool { return strings.HasPrefix(s, "Reload to start") }) {
			t.Fatalf("ready state was not shown: %q", states)
		}
		fill := requests.take()
		t.Logf("fill: %d requests, %d under react-dev/", len(fill), countReactDev(fill))

		// The reload boots development React from one generation.
		mute()
		reloadFromButton(t, page)
		waitForLiveApp(t, page)
		requireDevelopmentReact(t, page)
		// React prints its DevTools notice only in development builds, and only
		// for Chrome and Firefox user agents.
		if testHarness.browserName == "chromium" && !consoleDev() {
			t.Fatal("development React did not print its DevTools notice")
		}
		bar := page.GetByTestId("react-dev-status-button")
		if err := bar.WaitFor(); err != nil {
			t.Fatalf("Dev React bottom bar item did not appear: %v", err)
		}
		if text, err := bar.TextContent(); err != nil || strings.TrimSpace(text) != "Dev React" {
			t.Fatalf("bottom bar item reads %q: %v", text, err)
		}
	})

	t.Run("tampered", func(t *testing.T) {
		// WebKit does not route a service worker's fetches through Playwright,
		// so the changed byte would never reach the fill.
		if testHarness.browserName == "webkit" {
			t.Skip("WebKit service worker fetches bypass context routes")
		}

		// Change one byte of one development file on a fresh profile.
		ctx := testHarness.newBrowserContext(t)
		if err := ctx.Route("**/react-dev/pkgs/react-dom/client.mjs", func(route playwright.Route) {
			resp, err := route.Fetch()
			if err != nil {
				_ = route.Abort()
				return
			}
			body, err := resp.Body()
			if err != nil || len(body) == 0 {
				_ = route.Abort()
				return
			}
			body[len(body)-1] ^= 1
			_ = route.Fulfill(playwright.RouteFulfillOptions{Response: resp, Body: body})
		}); err != nil {
			t.Fatal(err)
		}
		page, mute := newDevReactPage(t, ctx)

		// The fill fails verification and offers a retry.
		openLocalSession(t, page)
		openDeveloperSettings(t, page)
		clickButton(t, page, "Turn on")
		waitButton(t, page, "Retry")
		requireStatus(t, page, "Development React could not be loaded")

		// The next boot stays on production React and still offers the retry.
		mute()
		if _, err := page.Reload(); err != nil {
			t.Fatal(err)
		}
		waitForLiveApp(t, page)
		requireProductionReact(t, page)
		openDeveloperSettings(t, page)
		waitButton(t, page, "Retry")
	})
}

// newDevReactPage opens a page with browser diagnostics and returns their
// mute. A reload tears down the plugin runtimes, which log their closed
// clients as console errors, so each subtest mutes diagnostics at its
// deliberate reload and checks the next document directly.
func newDevReactPage(t *testing.T, ctx playwright.BrowserContext) (playwright.Page, func()) {
	t.Helper()
	page, err := ctx.NewPage()
	if err != nil {
		t.Fatalf("new page: %v", err)
	}
	return page, testHarness.attachPageDiagnostics(t, page)
}

// requestLog collects request URLs until taken.
type requestLog struct {
	mu   sync.Mutex
	urls []string
}

// recordRequests logs every request in the context, including its service
// worker's.
func recordRequests(ctx playwright.BrowserContext) *requestLog {
	l := &requestLog{}
	ctx.OnRequest(func(req playwright.Request) {
		l.mu.Lock()
		l.urls = append(l.urls, req.URL())
		l.mu.Unlock()
	})
	return l
}

// take returns the URLs logged since the last take.
func (l *requestLog) take() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	urls := l.urls
	l.urls = nil
	return urls
}

// countReactDev counts URLs that read development React.
func countReactDev(urls []string) int {
	n := 0
	for _, u := range urls {
		if strings.Contains(u, "react-dev/") {
			n++
		}
	}
	return n
}

// writeRequestList saves the URLs, one per line, beside the run's artifacts.
func writeRequestList(t *testing.T, name string, urls []string) {
	t.Helper()
	path := filepath.Join(testHarness.artifactDir, "dev-react-"+testHarness.browserName+"-"+name+".txt")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Join(urls, "\n")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Logf("request list: %s", path)
}

// watchConsole reports whether any console message contained text.
func watchConsole(page playwright.Page, text string) func() bool {
	var mu sync.Mutex
	seen := false
	page.OnConsole(func(msg playwright.ConsoleMessage) {
		if strings.Contains(msg.Text(), text) {
			mu.Lock()
			seen = true
			mu.Unlock()
		}
	})
	return func() bool {
		mu.Lock()
		defer mu.Unlock()
		return seen
	}
}

// localSessionRoute matches the session route quickstart lands on.
var localSessionRoute = regexp.MustCompile(`^#/u/\d+/?$`)

// openLocalSession creates a local session through quickstart.
func openLocalSession(t *testing.T, page playwright.Page) {
	t.Helper()
	if _, err := page.Goto(testHarness.getBaseURL() + "/#/quickstart/local"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.WaitForFunction(`() => /^#\/u\/\d+\/?$/.test(location.hash)`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("local quickstart did not create a session: %v", err)
	}
	waitForLiveApp(t, page)
	if hash, _ := page.Evaluate(`() => location.hash`); !localSessionRoute.MatchString(hash.(string)) {
		t.Fatalf("unexpected session route %v", hash)
	}
}

// openDeveloperSettings opens the account menu's Developer section. Both the
// menu and the section may already be open, since the section's state persists.
func openDeveloperSettings(t *testing.T, page playwright.Page) {
	t.Helper()
	controls := page.GetByTestId("react-dev-controls")
	if visible, _ := controls.IsVisible(); visible {
		return
	}
	menu := page.GetByTestId("session-account-menu-button")
	if label, _ := menu.GetAttribute("aria-label"); label == "Open account menu" {
		if err := menu.Click(); err != nil {
			t.Fatalf("open account menu: %v", err)
		}
	}
	if err := button(page, "Developer").WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(60_000)}); err != nil {
		probe, _ := page.Evaluate(reactDevProbe)
		t.Fatalf("Developer section is absent: %v; page reads %v", err, probe)
	}
	if visible, _ := controls.IsVisible(); !visible {
		clickButton(t, page, "Developer")
	}
	if err := controls.WaitFor(); err != nil {
		t.Fatalf("Developer section did not show the setting: %v", err)
	}
}

// reactDevProbe reads what the Developer section depends on: a service worker
// API and a release the shell's template describes.
const reactDevProbe = `async () => {
	const out = { serviceWorker: 'serviceWorker' in navigator, controlled: !!navigator.serviceWorker?.controller }
	const t = document.getElementById('bldr-import-map')
	out.version = t?.getAttribute('data-react-version') ?? null
	out.templateChild = t?.content?.firstElementChild?.tagName ?? null
	try {
		const m = await import('@aptre/bldr')
		out.hasReader = typeof m.readReactDevRelease
		out.release = m.readReactDevRelease ? JSON.stringify(m.readReactDevRelease(document)) : null
	} catch (e) {
		out.importError = String(e)
	}
	out.storage = localStorage.getItem('bldr-react-dev')
	return JSON.stringify(out)
}`

// clickButton clicks the button with the exact accessible name.
func clickButton(t *testing.T, page playwright.Page, name string) {
	t.Helper()
	if err := button(page, name).Click(); err != nil {
		t.Fatalf("click %s: %v", name, err)
	}
}

// waitButton waits for the button with the exact accessible name.
func waitButton(t *testing.T, page playwright.Page, name string) {
	t.Helper()
	if err := button(page, name).WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(browserWaitMS)}); err != nil {
		t.Fatalf("%s did not appear: %v", name, err)
	}
}

// button locates a button by its exact accessible name.
func button(page playwright.Page, name string) playwright.Locator {
	return page.GetByRole("button", playwright.PageGetByRoleOptions{Name: name, Exact: new(true)}).First()
}

// reactDevStatus is the setting's status line.
const reactDevStatus = `[data-testid="react-dev-controls"] [role="status"]`

// watchReactDevStatus records each distinct status line in the page.
func watchReactDevStatus(t *testing.T, page playwright.Page) {
	t.Helper()
	if _, err := page.Evaluate(`sel => {
		const states = globalThis.__reactDevStates = []
		const note = () => {
			const text = document.querySelector(sel)?.textContent?.trim() ?? ''
			if (text && states[states.length - 1] !== text) states.push(text)
		}
		note()
		new MutationObserver(note).observe(document.body, { subtree: true, childList: true, characterData: true })
	}`, reactDevStatus); err != nil {
		t.Fatal(err)
	}
}

// readReactDevStatus returns the status lines recorded so far.
func readReactDevStatus(t *testing.T, page playwright.Page) []string {
	t.Helper()
	raw, err := page.Evaluate(`() => globalThis.__reactDevStates ?? []`)
	if err != nil {
		t.Fatal(err)
	}
	var states []string
	for _, s := range raw.([]any) {
		states = append(states, s.(string))
	}
	return states
}

// requireStatus checks the status line starts with text.
func requireStatus(t *testing.T, page playwright.Page, text string) {
	t.Helper()
	got, err := page.Locator(reactDevStatus).TextContent()
	if err != nil || !strings.HasPrefix(strings.TrimSpace(got), text) {
		t.Fatalf("status reads %q, want %q: %v", got, text, err)
	}
}

// reloadFromButton clicks Reload and waits for the next document.
func reloadFromButton(t *testing.T, page playwright.Page) {
	t.Helper()
	if _, err := page.Evaluate(`() => { globalThis.__reactDevOldDocument = true }`); err != nil {
		t.Fatal(err)
	}
	clickButton(t, page, "Reload")
	if _, err := page.WaitForFunction(`() => !globalThis.__reactDevOldDocument && document.readyState === 'complete'`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("Reload did not load a new document: %v", err)
	}
}

// reactSources reads the shell's marker, the import map's React URLs and the
// React files the document loaded.
const reactSources = `() => {
	const map = JSON.parse(document.querySelector('script[type="importmap"]')?.textContent ?? '{"imports":{}}')
	const mapped = Object.entries(map.imports ?? {}).filter(([k]) => /^react(-dom)?(\/|$)/.test(k)).map(([, v]) => v)
	const loaded = performance.getEntriesByType('resource').map(e => e.name).filter(n => /\/pkgs\/react(-dom)?\//.test(n))
	return { marker: globalThis.__bldrReactDev ?? '', mapped, loaded }
}`

// readReactSources evaluates reactSources.
func readReactSources(t *testing.T, page playwright.Page) (string, []string, []string) {
	t.Helper()
	raw, err := page.Evaluate(reactSources)
	if err != nil {
		t.Fatal(err)
	}
	m := raw.(map[string]any)
	strs := func(v any) []string {
		var out []string
		for _, s := range v.([]any) {
			out = append(out, s.(string))
		}
		return out
	}
	return m["marker"].(string), strs(m["mapped"]), strs(m["loaded"])
}

// requireProductionReact checks the document maps and loads only production
// React.
func requireProductionReact(t *testing.T, page playwright.Page) {
	t.Helper()
	marker, mapped, loaded := readReactSources(t, page)
	if marker != "" || len(mapped) == 0 || countReactDev(mapped) != 0 || countReactDev(loaded) != 0 {
		t.Fatalf("want production React: marker=%q mapped=%q loaded=%q", marker, mapped, loaded)
	}
}

// requireDevelopmentReact checks the shell chose development React and every
// React file the document loaded came from that one generation.
func requireDevelopmentReact(t *testing.T, page playwright.Page) {
	t.Helper()
	marker, mapped, loaded := readReactSources(t, page)
	t.Logf("development React %s: %d mapped, %d loaded", marker, len(mapped), len(loaded))
	if marker == "" || len(mapped) == 0 || countReactDev(mapped) != len(mapped) {
		t.Fatalf("want development React: marker=%q mapped=%q", marker, mapped)
	}
	if len(loaded) == 0 || countReactDev(loaded) != len(loaded) {
		t.Fatalf("React files outside the development generation: %q", loaded)
	}
}
