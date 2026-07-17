//go:build !js

package wasm

import (
	"strconv"
	"strings"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
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
	QuickstartTiming          map[string]any
}

// WaitForApp waits for the real app runtime, not the prerendered shell, to be
// connected to the Resource SDK.
func WaitForApp(t testing.TB, page playwright.Page) {
	t.Helper()

	deadlineMS := 120000
	if E2EWasmSlowCompilerEnabled() {
		deadlineMS = 240000
	}

	deadline := time.Now().Add(time.Duration(deadlineMS) * time.Millisecond)
	var lastErr error
	for time.Now().Before(deadline) {
		remainingMS := time.Until(deadline).Milliseconds()
		if remainingMS <= 0 {
			break
		}
		evalWindowMS := min(remainingMS, int64(10000))
		_, err := page.Evaluate(`async ({ deadlineMS }) => {
		const deadline = performance.now() + deadlineMS
		let booted = false
		let readyResolved = !globalThis.__swReady
		let readyRejected = null
		if (globalThis.__swReady?.then) {
			globalThis.__swReady.then(
				() => {
					readyResolved = true
				},
				(err) => {
					readyRejected = String(err)
				},
			)
		}
		while (!globalThis.__s4wave_debug?.root) {
			if (!booted && readyResolved && typeof globalThis.__swBoot === 'function') {
				globalThis.__swBoot(window.location.hash || '#/')
				booted = true
			}
			if (performance.now() > deadline) {
				const state = JSON.stringify({
					booted,
					hasBoot: typeof globalThis.__swBoot === 'function',
					hasReady: !!globalThis.__swReady,
					readyResolved,
					readyRejected,
					bootStatus: globalThis.__swBootStatus ?? null,
					startupMarks: globalThis.__swStartupMarks ?? [],
				})
				throw new Error('debug context did not initialize before deadline: ' + state)
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
		return null
	}`, map[string]any{"deadlineMS": evalWindowMS})
		if err == nil {
			return
		}
		lastErr = err
		if !isTransientAppWaitError(err) {
			break
		}
		time.Sleep(250 * time.Millisecond)
	}

	body, bodyErr := page.Locator("body").TextContent()
	if bodyErr != nil {
		body = "failed to read body text: " + bodyErr.Error()
	}
	t.Fatalf(
		"app not ready: %v\nurl: %s\nbody: %s",
		lastErr,
		page.URL(),
		trimPageText(body),
	)
}

func isTransientAppWaitError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "execution context was destroyed") ||
		strings.Contains(msg, "most likely because of a navigation") ||
		strings.Contains(msg, "cannot find context with specified id") ||
		strings.Contains(msg, "target closed") ||
		strings.Contains(msg, "debug context did not initialize before deadline")
}

func AssertBrowserStartupDone(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	proof := readRuntimeStartupProof(t, h, page)
	if got := stringField(proof, "phaseId"); got != "done" {
		t.Fatalf("browser startup phase=%q want done; proof=%#v", got, proof)
	}
	if got := stringField(proof, "viewState"); got != "synced" {
		t.Fatalf("browser startup view state=%q want synced; proof=%#v", got, proof)
	}
	runtime := mapField(t, proof, "runtime")
	if terminalFailure := runtime["terminalFailure"]; terminalFailure != nil {
		t.Fatalf("browser startup has terminal failure: %#v", terminalFailure)
	}
	if got := stringField(runtime, "runtimeClientState"); got != "connected" {
		t.Fatalf("browser runtime client state=%q want connected; proof=%#v", got, proof)
	}
	frameState := stringField(runtime, "frameState")
	if frameState != "revealed" {
		t.Fatalf("browser frame state=%q want revealed; proof=%#v", frameState, proof)
	}
	return proof
}

func AssertRootImportMap(t testing.TB, h *Harness, page playwright.Page) {
	t.Helper()

	proof := readRuntimeStartupProof(t, h, page)
	importMap := mapField(t, proof, "importMap")
	if !boolField(importMap, "hasReact") {
		t.Fatalf("root import map missing react specifier; proof=%#v", proof)
	}
	if !boolField(importMap, "hasReactDomClient") {
		t.Fatalf("root import map missing react-dom/client specifier; proof=%#v", proof)
	}
	if !boolField(importMap, "hasProtobufServiceType") {
		t.Fatalf("root import map missing @aptre/protobuf-es-lite/service-type specifier; proof=%#v", proof)
	}
	if got := intField(importMap, "importCount"); got == 0 {
		t.Fatalf("root import map is empty; proof=%#v", proof)
	}
}

func readRuntimeStartupProof(t testing.TB, h *Harness, page playwright.Page) map[string]any {
	t.Helper()

	raw, err := page.Evaluate(h.Script("runtime-startup-state.ts"), nil)
	if err != nil {
		t.Fatalf("read runtime startup proof: %v", err)
	}
	proof, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected runtime startup proof %T: %#v", raw, raw)
	}
	return proof
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

func visibleDriveBrowser(page playwright.Page) playwright.Locator {
	return page.Locator("[data-testid='unixfs-browser']:visible").First()
}

// WaitForDriveShell waits for the drive viewer shell to render.
func WaitForDriveShell(t testing.TB, page playwright.Page) {
	t.Helper()

	CompleteDriveIntroWizardIfPresent(t, page)
	err := visibleDriveBrowser(page).WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	)
	if err != nil {
		body, bodyErr := page.Locator("body").TextContent()
		if bodyErr != nil {
			body = "failed to read body text: " + bodyErr.Error()
		}
		debug, debugErr := page.Evaluate(`async () => {
			async function firstStreamValue(stream, signal) {
				for await (const value of stream) {
					return value
				}
				return null
			}
			async function routeProbe() {
				const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
				const root = globalThis.__s4wave_debug?.root
				if (!match || !root) {
					return { skipped: true, hasDebugRoot: !!root }
				}
				const sessionIdx = Number(match[1])
				const sharedObjectId = decodeURIComponent(match[2])
				let session = null
				let sharedObject = null
				let body = null
				let space = null
				try {
					const abort = AbortSignal.timeout(15000)
					const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
					session = mounted?.session ?? null
					if (!session) return { skipped: false, session: false }
					sharedObject = await session.mountSharedObject({ sharedObjectId }, abort)
					if (!sharedObject) return { skipped: false, session: true, sharedObject: false }
					body = await sharedObject.mountSharedObjectBody({}, abort)
					const { Space } = await import('@s4wave/sdk/space/space.js')
					space = new Space(body.resourceRef.createRef(body.id))
					const state = await firstStreamValue(space.watchSpaceState({}, abort), abort)
					return {
						skipped: false,
						session: true,
						sharedObject: true,
						body: true,
						spaceState: state ? {
							ready: !!state.ready,
							indexPath: state.settings?.indexPath ?? '',
							objectKeys: (state.worldContents?.objects ?? []).map((obj) => obj.objectKey ?? ''),
						} : null,
					}
				} catch (err) {
					return { skipped: false, error: String(err?.stack ?? err) }
				} finally {
					space?.release?.()
					body?.release?.()
					sharedObject?.release?.()
					session?.release?.()
				}
			}
			const headings = Array.from(document.querySelectorAll('h1,h2,h3,[data-slot="loading-title"],[data-slot="loading-detail"]')).map((el) => ({
				tag: el.tagName,
				text: el.textContent?.slice(0, 160) ?? '',
			}))
			return JSON.stringify({
				hash: window.location.hash,
				hasDebugRoot: !!globalThis.__s4wave_debug?.root,
				quickstartTiming: globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null,
				routeProbe: await routeProbe(),
				bodyHtml: document.body.innerHTML.slice(0, 3000),
				text: document.body.textContent?.slice(0, 1500) ?? '',
				headings,
				links: Array.from(document.querySelectorAll('link')).map((link) => ({
					href: link.href,
					rel: link.rel,
					loaded: !!link.sheet,
				})),
				testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
					testid: el.getAttribute('data-testid'),
					text: el.textContent?.slice(0, 120) ?? '',
				})),
			})
		}`)
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

// WaitForEmptySpaceReady waits for the static Space quickstart root route to
// render the empty Space affordance.
func WaitForEmptySpaceReady(t testing.TB, page playwright.Page) {
	t.Helper()

	for _, selector := range []string{
		"text=Empty Space",
		"text=Create your first object",
	} {
		if err := page.Locator(selector).First().WaitFor(
			playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
		); err != nil {
			body, bodyErr := page.Locator("body").TextContent()
			if bodyErr != nil {
				body = "failed to read body text: " + bodyErr.Error()
			}
			debug, debugErr := page.Evaluate(`async () => {
				const timing = globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null
				return JSON.stringify({
					hash: window.location.hash,
					hasDebugRoot: !!globalThis.__s4wave_debug?.root,
					startup: globalThis.__swBootStatus ?? null,
					startupMarks: globalThis.__swStartupMarks ?? [],
					timing,
					testIds: Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
						testid: el.getAttribute('data-testid'),
						text: el.textContent?.slice(0, 180) ?? '',
						tag: el.tagName,
					})),
					buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
						text: button.textContent?.slice(0, 180) ?? '',
						ariaLabel: button.getAttribute('aria-label') ?? '',
						disabled: button.disabled,
					})),
					bodyHtml: document.body.innerHTML.slice(0, 5000),
					bodyText: document.body.textContent?.slice(0, 2000) ?? '',
				})
			}`)
			if debugErr != nil {
				debug = "failed to collect page debug: " + debugErr.Error()
			}
			t.Fatalf(
				"wait for empty Space selector %q: %v\nurl: %s\nbody: %s\ndebug: %v",
				selector,
				err,
				page.URL(),
				trimPageText(body),
				debug,
			)
		}
	}

	raw, err := page.Evaluate(`() => {
		const timing = globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null
		return {
			hash: window.location.hash,
			quickstartState: timing?.state ?? '',
			quickstartError: timing?.error ?? '',
		}
	}`, nil)
	if err != nil {
		t.Fatalf("read empty Space ready state: %v", err)
	}
	state, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("unexpected empty Space ready state %T: %#v", raw, raw)
	}
	hash := stringField(state, "hash")
	if !strings.Contains(hash, "#/u/") || !strings.Contains(hash, "/so/") {
		t.Fatalf("empty Space did not reach direct Space route: %#v", state)
	}
	if errMsg := stringField(state, "quickstartError"); errMsg != "" {
		t.Fatalf("empty Space quickstart timing recorded an error: %#v", state)
	}
	if got := stringField(state, "quickstartState"); got != "" && got != "content-ready" {
		t.Fatalf("empty Space quickstart state=%q want content-ready: %#v", got, state)
	}
}

const completeDriveIntroWizardScript = `async () => {
	const deadline = Date.now() + 120000
	const actionLabels = ['Next', 'Got it, start exploring', 'Open files']
	for (;;) {
		let action = null
		for (const button of document.querySelectorAll('button')) {
			if (!button.disabled && actionLabels.includes(button.textContent?.trim() ?? '')) {
				action = button
				break
			}
		}
		if (action) {
			action.click()
			await new Promise((resolve) => requestAnimationFrame(resolve))
			continue
		}
		const browser = document.querySelector('[data-testid="unixfs-browser"]')
		if (browser && !window.location.hash.includes('/wizard/')) return null
		if (Date.now() > deadline) {
			throw new Error('Drive intro or file browser did not appear')
		}
		await new Promise((resolve) => requestAnimationFrame(resolve))
	}
}`

// CompleteDriveIntroWizardIfPresent completes the first-run Drive intro when
// the current route opens it before the raw files browser.
func CompleteDriveIntroWizardIfPresent(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(completeDriveIntroWizardScript)
	if err != nil {
		body, bodyErr := page.Locator("body").TextContent()
		if bodyErr != nil {
			body = "failed to read body text: " + bodyErr.Error()
		}
		debug, debugErr := page.Evaluate(`async () => {
			async function firstStreamValue(stream) {
				for await (const value of stream) {
					return value
				}
				return null
			}
			async function routeProbe() {
				const match = window.location.hash.match(/^#\/u\/([0-9]+)\/so\/([^/]+)/)
				const root = globalThis.__s4wave_debug?.root
				if (!match || !root) {
					return { skipped: true, hasDebugRoot: !!root }
				}
				const sessionIdx = Number(match[1])
				const sharedObjectId = decodeURIComponent(match[2])
				let session = null
				let sharedObject = null
				let body = null
				let space = null
				let step = 'mountSessionByIdx'
				try {
					const abort = AbortSignal.timeout(15000)
					const mounted = await root.mountSessionByIdx({ sessionIdx }, abort)
					session = mounted?.session ?? null
					if (!session) return { skipped: false, session: false }
					step = 'mountSharedObject'
					sharedObject = await session.mountSharedObject({ sharedObjectId }, abort)
					if (!sharedObject) return { skipped: false, session: true, sharedObject: false }
					step = 'mountSharedObjectBody'
					body = await sharedObject.mountSharedObjectBody({}, abort)
					step = 'importSpace'
					const { Space } = await import('@s4wave/sdk/space/space.js')
					space = new Space(body.resourceRef.createRef(body.id))
					step = 'watchSpaceState'
					const state = await firstStreamValue(space.watchSpaceState({}, abort))
					return {
						skipped: false,
						step,
						session: true,
						sharedObject: true,
						body: true,
						spaceState: state ? {
							ready: !!state.ready,
							indexPath: state.settings?.indexPath ?? '',
							objectKeys: (state.worldContents?.objects ?? []).map((obj) => obj.objectKey ?? ''),
						} : null,
					}
				} catch (err) {
					return { skipped: false, step, error: String(err?.stack ?? err) }
				} finally {
					space?.release?.()
					body?.release?.()
					sharedObject?.release?.()
					session?.release?.()
				}
			}
			const startup = globalThis.__swBootStatus ?? null
			const timing = globalThis.__s4waveQuickstartTiming ?? globalThis.__s4wave_debug?.quickstartTiming ?? null
			const hash = window.location.hash
			const testIds = Array.from(document.querySelectorAll('[data-testid]')).map((el) => ({
				testid: el.getAttribute('data-testid'),
				text: el.textContent?.slice(0, 180) ?? '',
				tag: el.tagName,
			}))
			const buttons = Array.from(document.querySelectorAll('button')).map((button) => ({
				text: button.textContent?.slice(0, 180) ?? '',
				disabled: button.disabled,
			}))
			const headings = Array.from(document.querySelectorAll('h1,h2,h3,[data-slot="loading-title"],[data-slot="loading-detail"]')).map((el) => ({
				tag: el.tagName,
				text: el.textContent?.slice(0, 180) ?? '',
			}))
			return JSON.stringify({
				hash,
				hasDebugRoot: !!globalThis.__s4wave_debug?.root,
				startup,
				startupMarks: globalThis.__swStartupMarks ?? [],
				timing,
				testIds,
				buttons,
				headings,
				routeProbe: await routeProbe(),
				bodyHtml: document.body.innerHTML.slice(0, 5000),
				bodyText: document.body.textContent?.slice(0, 2000) ?? '',
			})
		}`)
		if debugErr != nil {
			debug = "failed to collect page debug: " + debugErr.Error()
		}
		t.Fatalf("complete drive intro if present: %v\nurl: %s\nbody: %s\ndebug: %v", err, page.URL(), trimPageText(body), debug)
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
		result.QuickstartTiming = timing
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

func boolField(m map[string]any, key string) bool {
	v, _ := m[key].(bool)
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

func mapField(t testing.TB, m map[string]any, key string) map[string]any {
	t.Helper()

	v, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("expected map field %q in %#v", key, m)
	}
	return v
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
	return strconv.Itoa(*v) + "ms"
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
