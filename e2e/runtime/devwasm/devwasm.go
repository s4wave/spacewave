//go:build !js

// Package devwasm adapts the existing browser WASM harness to the scenario
// runtime contract.
package devwasm

import (
	"context"
	"strings"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/e2e/runtime"
	"github.com/s4wave/spacewave/e2e/wasm"
	"github.com/sirupsen/logrus"
)

const defaultWait = 120 * time.Second

// Options configures the dev WASM runtime adapter.
type Options struct{}

// Adapter drives scenarios through the existing browser WASM harness.
type Adapter struct {
	h                 *wasm.Harness
	ctx               playwright.BrowserContext
	page              playwright.Page
	resetDriveOnReady bool
}

// New boots the browser WASM harness and opens the first scenario page.
func New(ctx context.Context, _ Options) (*Adapter, error) {
	compiler, err := wasm.ResolveE2EWasmCompiler()
	if err != nil {
		return nil, errors.Wrap(err, "resolve devwasm compiler")
	}
	bootOptions := []wasm.Option{
		wasm.WithSessionHarness(),
		wasm.WithManifestBuildTimeout(20 * time.Minute),
	}
	switch compiler {
	case wasm.E2EWasmCompilerTinyGo:
		if err := wasm.ApplyE2EWasmTinyGoCompilerEnv(); err != nil {
			return nil, errors.Wrap(err, "configure TinyGo compiler")
		}
		bootOptions = append(bootOptions, wasm.WithTinyGoCore())
	case wasm.E2EWasmCompilerGoScript:
		bootOptions = append(bootOptions, wasm.WithGoScriptBrowserStartup())
	}
	h, err := wasm.Boot(ctx, logrus.NewEntry(logrus.New()), bootOptions...)
	if err != nil {
		return nil, errors.Wrap(err, "boot devwasm harness")
	}
	if err := h.LaunchBrowser(); err != nil {
		h.Release()
		return nil, errors.Wrap(err, "launch devwasm browser")
	}
	a := &Adapter{h: h}
	if err := a.newPage(); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

// Name returns the report identifier for the dev WASM runtime.
func (a *Adapter) Name() string { return "devwasm" }

// Close releases the browser context and shared harness.
func (a *Adapter) Close() {
	if a.ctx != nil {
		_ = a.ctx.Close()
		a.ctx = nil
	}
	if a.h != nil {
		a.h.Release()
		a.h = nil
	}
}

func (a *Adapter) newPage() error {
	var err error
	if a.ctx == nil {
		a.ctx, err = a.h.Browser().NewContext(playwright.BrowserNewContextOptions{AcceptDownloads: new(true)})
		if err != nil {
			return errors.Wrap(err, "create browser context")
		}
	}
	a.page, err = a.ctx.NewPage()
	if err != nil {
		return errors.Wrap(err, "create browser page")
	}
	return nil
}

// ResetSession applies a declared scenario boundary without rebooting the
// harness or browser process.
func (a *Adapter) ResetSession(requirement runtime.SessionRequirement) error {
	switch requirement {
	case runtime.SessionFresh:
		if a.page != nil {
			if err := a.page.Close(); err != nil {
				return errors.Wrap(err, "close session page")
			}
			a.page = nil
		}
		return a.newPage()
	case runtime.SessionFreshInstall:
		if a.ctx != nil {
			if err := a.ctx.Close(); err != nil {
				return errors.Wrap(err, "close install context")
			}
			a.ctx = nil
			a.page = nil
		}
		return a.newPage()
	default:
		return errors.Errorf("unsupported session requirement %q", requirement)
	}
}

// OpenRoute opens route through a document load or committed client-side hash.
func (a *Adapter) OpenRoute(route string) error {
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	if needsDocumentLoad(a.page.URL()) {
		if route == "/quickstart/drive" {
			a.resetDriveOnReady = true
		}
		if _, err := a.page.Goto(a.h.BaseURL() + "/#" + route); err != nil {
			return errors.Wrapf(err, "open route %q", route)
		}
		return a.WaitForEvent(runtime.EventAppReady)
	}
	targetHash := warmRouteHash(route)
	a.resetDriveOnReady = route == "/quickstart/drive" && pageHash(a.page.URL()) != targetHash
	if err := a.openRouteClientSide(targetHash); err != nil {
		return errors.Wrapf(err, "open route %q client-side", route)
	}
	return a.WaitForEvent(runtime.EventAppReady)
}

func (a *Adapter) openRouteClientSide(targetHash string) error {
	_, err := a.page.WaitForFunction(`({ targetHash }) => {
		const eventName = 's4wave:shell-tab-path-committed'
		const targetPath = targetHash.startsWith('#') ? targetHash.slice(1) : targetHash
		if (window.location.hash === targetHash) return true
		return new Promise((resolve, reject) => {
			const markerKey = '__s4waveE2EOpenRouteMarker'
			const marker = String(Date.now()) + ':' + String(Math.random())
			const cleanup = () => {
				window.removeEventListener(eventName, onCommit)
				delete window[markerKey]
			}
			const onCommit = (event) => {
				if (window[markerKey] !== marker) {
					cleanup()
					reject(new Error('warm navigation marker did not survive route change'))
					return
				}
				const detail = event instanceof CustomEvent ? event.detail : null
				if (detail?.path !== targetPath || window.location.hash !== targetHash) return
				cleanup()
				resolve(true)
			}
			window[markerKey] = marker
			window.addEventListener(eventName, onCommit)
			window.location.hash = targetHash
		})
	}`, map[string]any{"targetHash": targetHash}, playwright.PageWaitForFunctionOptions{Timeout: durationMS(defaultWait)})
	return err
}

func (a *Adapter) waitForNextTabPathCommit(action string, run func() error) error {
	const key = "__s4waveE2ENextTabPathCommit"
	if _, err := a.page.Evaluate(`({ key }) => {
		const eventName = 's4wave:shell-tab-path-committed'
		window[key]?.cleanup?.()
		let cleanup = () => {}
		const promise = new Promise((resolve) => {
			const onCommit = (event) => {
				cleanup()
				const detail = event instanceof CustomEvent ? event.detail : null
				resolve(detail?.path ?? null)
			}
			cleanup = () => {
				window.removeEventListener(eventName, onCommit)
			}
			window.addEventListener(eventName, onCommit)
		})
		window[key] = { promise, cleanup }
	}`, map[string]any{"key": key}); err != nil {
		return errors.Wrapf(err, "arm tab path commit wait before %s", action)
	}
	if err := run(); err != nil {
		_, _ = a.page.Evaluate(`({ key }) => {
			window[key]?.cleanup?.()
			delete window[key]
		}`, map[string]any{"key": key})
		return err
	}
	_, err := a.page.WaitForFunction(`({ key }) => window[key]?.promise`, map[string]any{"key": key}, playwright.PageWaitForFunctionOptions{Timeout: durationMS(defaultWait)})
	_, _ = a.page.Evaluate(`({ key }) => delete window[key]`, map[string]any{"key": key})
	if err != nil {
		return errors.Wrapf(err, "wait for tab path commit after %s", action)
	}
	return nil
}

func needsDocumentLoad(pageURL string) bool {
	return pageURL == "" || pageURL == "about:blank"
}

func pageHash(pageURL string) string {
	hashIndex := strings.IndexByte(pageURL, '#')
	if hashIndex < 0 {
		return ""
	}
	hash := pageURL[hashIndex:]
	if queryIndex := strings.IndexByte(hash, '?'); queryIndex >= 0 {
		hash = hash[:queryIndex]
	}
	return hash
}

// warmRouteHash returns the route form consumed by the app's hash router.
// Session prefixes are resolved by the outer router, not the session route.
func warmRouteHash(route string) string {
	return "#" + route
}

func (a *Adapter) driveBrowser() playwright.Locator {
	return a.page.Locator("[data-testid='unixfs-browser']:visible").Last()
}

// ClickControl activates a named Drive control or Playwright selector.
func (a *Adapter) ClickControl(control string) error {
	if control == "confirm" {
		return a.driveBrowser().Locator("input[placeholder='Folder name']:visible").First().Press("Enter")
	}
	if control == "new-folder" {
		browser := a.driveBrowser()
		input := browser.Locator("input[placeholder='Folder name']:visible").First()
		buttons := browser.Locator("button[title='New folder']:not([disabled]):visible")
		// Count is a snapshot. Right after navigating into a folder the browser
		// that driveBrowser resolves is the newly mounted one, whose toolbar has
		// not rendered its actions yet, so the snapshot can be zero. That left
		// the loop below with nothing to click and spent the whole wait on an
		// input no click had opened. Wait for a clickable button first.
		if err := buttons.First().WaitFor(playwright.LocatorWaitForOptions{
			Timeout: durationMS(defaultWait),
		}); err != nil {
			return err
		}
		count, err := buttons.Count()
		if err != nil {
			return err
		}
		for index := range count {
			if err := buttons.Nth(index).Click(); err != nil {
				continue
			}
			visible, err := input.IsVisible()
			if err != nil {
				return err
			}
			if visible {
				return nil
			}
		}
		return input.WaitFor(playwright.LocatorWaitForOptions{Timeout: durationMS(defaultWait)})
	}
	selectors := map[string]string{
		"drive":        "text=Create a Drive",
		"up":           "button[title='Up']:not([disabled]):visible",
		"home":         "button[aria-label='Navigate to root']:visible, button[title='Home']:not([disabled]):visible",
		"back":         "button[title='Back']:not([disabled]):visible",
		"forward":      "button[title='Forward']:not([disabled]):visible",
		"delete-space": "button:has-text('Delete Space'):visible",
	}
	selector := selectors[control]
	if selector == "" {
		selector = control
	}
	if control == "drive" || control == "delete-space" {
		return a.page.Locator(selector).Last().Click()
	}
	if control == "up" || control == "home" || control == "back" || control == "forward" {
		return a.waitForNextTabPathCommit(control, func() error {
			return a.driveBrowser().Locator(selector).First().Click()
		})
	}
	return a.driveBrowser().Locator(selector).First().Click()
}

// DoubleClickContent opens a visible row by text.
func (a *Adapter) DoubleClickContent(content string) error {
	return a.waitForNextTabPathCommit("open "+content, func() error {
		return a.driveBrowser().Locator("[role='row']:has-text('" + content + "')").First().Dblclick()
	})
}

// Type fills a named placeholder field or selector.
func (a *Adapter) Type(field string, value string) error {
	selector := field
	if !strings.ContainsAny(field, "[]#.") {
		selector = "input[placeholder='" + field + "']"
	}
	return a.page.Locator(selector + ":visible").Last().Fill(value)
}

// UploadFile supplies an in-memory fixture to a file input.
func (a *Adapter) UploadFile(target string, file runtime.File) error {
	selector := target
	if selector == "" {
		selector = "input[type='file']"
	}
	return a.page.Locator(selector).Last().SetInputFiles([]playwright.InputFile{{
		Name:     file.Name,
		MimeType: file.MIMEType,
		Buffer:   file.Contents,
	}})
}

// MoveContent drops source content onto target content.
func (a *Adapter) MoveContent(source string, target string) error {
	_, err := a.page.Evaluate(`({ source, target }) => {
		const isVisible = (element) => {
			const style = getComputedStyle(element)
			return style.display !== 'none' && style.visibility !== 'hidden' && element.getClientRects().length > 0
		}
		const browser = Array.from(document.querySelectorAll('[data-testid="unixfs-browser"]')).find(isVisible)
		if (!(browser instanceof HTMLElement)) throw new Error('visible Drive browser not found')
		const row = Array.from(browser.querySelectorAll('[role="row"]')).find((el) => el.textContent?.includes(target) && isVisible(el))
		if (!(row instanceof HTMLElement)) throw new Error('target row not found: ' + target)
		const transfer = new DataTransfer()
		transfer.setData('application/x-s4wave-app-drag+json', JSON.stringify({version: 1, items: [{id: source, label: source, capabilities: [{kind: 'movable', value: {case: 'unixfs-entry', value: {unixfsId: 'files', path: '/' + source, isDir: false}}}]}]}))
		for (const type of ['dragover', 'drop']) row.dispatchEvent(new DragEvent(type, {bubbles: true, cancelable: true, dataTransfer: transfer}))
	}`, map[string]any{"source": source, "target": target})
	return err
}

// ExpectVisible waits for visible text content to appear.
func (a *Adapter) ExpectVisible(content string) error {
	return a.waitForText(content, true)
}

// ExpectAbsent waits for visible text content to disappear.
func (a *Adapter) ExpectAbsent(content string) error {
	return a.waitForText(content, false)
}

func (a *Adapter) waitForText(content string, wantVisible bool) error {
	_, err := a.page.WaitForFunction(`({ content, wantVisible }) => {
		const isVisible = (element) => {
			const style = getComputedStyle(element)
			const rects = element.getClientRects()
			return style.display !== 'none' && style.visibility !== 'hidden' && rects.length > 0
		}
		const hasVisibleText = () => Array.from(document.querySelectorAll('body *')).some((element) =>
			element.textContent?.includes(content) && isVisible(element),
		)
		const matches = () => hasVisibleText() === wantVisible
		if (matches()) return true
		return new Promise((resolve) => {
			const observer = new MutationObserver(() => {
				if (!matches()) return
				observer.disconnect()
				resolve(true)
			})
			observer.observe(document.body, {
				subtree: true,
				childList: true,
				attributes: true,
				characterData: true,
			})
		})
	}`, map[string]any{"content": content, "wantVisible": wantVisible}, playwright.PageWaitForFunctionOptions{Timeout: durationMS(defaultWait)})
	return err
}

// DeleteSpace deletes the current Space and waits for the dialog to close and
// the Space name to leave the observed UI state.
func (a *Adapter) DeleteSpace() error {
	menu := a.page.Locator("[role='button'][aria-label='Open shared object menu']:visible").Last()
	if err := menu.Click(); err != nil {
		snapshot, snapshotErr := a.page.Evaluate(`() => ({
			url: window.location.href,
			browser: Boolean(document.querySelector('[data-testid="unixfs-browser"]')),
			buttons: Array.from(document.querySelectorAll('button'))
				.filter((button) => button.getClientRects().length > 0)
				.map((button) => ({
					aria: button.getAttribute('aria-label'),
					title: button.getAttribute('title'),
					text: button.textContent?.trim(),
				})),
		})`)
		if snapshotErr == nil {
			return errors.Wrapf(err, "open shared object menu (state: %v)", snapshot)
		}
		return errors.Wrap(err, "open shared object menu")
	}
	danger := a.page.Locator("button").Filter(playwright.LocatorFilterOptions{HasText: "Danger Zone"}).Last()
	if err := danger.Click(); err != nil {
		return errors.Wrap(err, "open danger zone")
	}
	trigger := a.page.Locator("button").Filter(playwright.LocatorFilterOptions{HasText: "Delete Object"}).Last()
	if err := trigger.Click(); err != nil {
		return errors.Wrap(err, "open delete space dialog")
	}
	input := a.page.Locator("[role='dialog'] input:visible").First()
	spaceName, err := input.GetAttribute("placeholder")
	if err != nil {
		return errors.Wrap(err, "read delete space confirmation name")
	}
	if spaceName == "" {
		return errors.New("delete space confirmation has no space name")
	}
	if err := input.Fill(spaceName); err != nil {
		return errors.Wrap(err, "confirm delete space")
	}
	if err := a.page.Locator("[role='dialog'] button:has-text('Delete Space'):visible").Last().Click(); err != nil {
		return errors.Wrap(err, "submit delete space")
	}
	return a.waitForSpaceDeleted(spaceName)
}

func (a *Adapter) waitForSpaceDeleted(spaceName string) error {
	_, err := a.page.WaitForFunction(`({ spaceName }) => {
		const isVisible = (element) => {
			const style = getComputedStyle(element)
			return style.display !== 'none' && style.visibility !== 'hidden' && element.getClientRects().length > 0
		}
		const textMatches = (element) => (element.textContent?.trim() ?? '').includes(spaceName)
		const spaceGone = () => !Array.from(document.querySelectorAll('[role="row"], [data-testid="space-list"] *, [data-testid="resource-list"] *, button, a'))
			.some((element) => isVisible(element) && textMatches(element))
		const done = () => !document.querySelector('[role="dialog"]') && spaceGone()
		if (done()) return true
		return new Promise((resolve) => {
			const observer = new MutationObserver(() => {
				if (!done()) return
				observer.disconnect()
				resolve(true)
			})
			observer.observe(document.body, {
				subtree: true,
				childList: true,
				attributes: true,
				characterData: true,
			})
		})
	}`, map[string]any{"spaceName": spaceName}, playwright.PageWaitForFunctionOptions{Timeout: durationMS(defaultWait)})
	return err
}

// ExpectRoute verifies that the current URL contains route.
func (a *Adapter) ExpectRoute(route string) error {
	if !strings.Contains(a.page.URL(), route) {
		return errors.Errorf("expected route %q, got %q", route, a.page.URL())
	}
	return nil
}

// WaitForEvent waits for a named app readiness transition.
func (a *Adapter) WaitForEvent(event runtime.Event) error {
	if event == runtime.EventDriveReady {
		var introErr error
		for range 3 {
			_, introErr = a.page.Evaluate(`async () => {
				const deadline = Date.now() + 120000
				const labels = ['Next', 'Got it, start exploring', 'Open files']
				const isVisible = (element) => {
					const style = getComputedStyle(element)
					return style.display !== 'none' && style.visibility !== 'hidden' && element.getClientRects().length > 0
				}
				for (;;) {
					const action = Array.from(document.querySelectorAll('button')).find((button) =>
						!button.disabled &&
						labels.includes(button.textContent?.trim() ?? '') &&
						isVisible(button),
					)
					if (action instanceof HTMLElement) {
						action.click()
						await new Promise((resolve) => requestAnimationFrame(resolve))
						continue
					}
					const browser = document.querySelector('[data-testid="unixfs-browser"]')
					if (browser && !window.location.hash.includes('/wizard/')) return null
					if (Date.now() > deadline) throw new Error('Drive intro or file browser did not appear')
					await new Promise((resolve) => requestAnimationFrame(resolve))
				}
			}`)
			if introErr == nil || !isTransientNavigationError(introErr.Error()) {
				break
			}
			if err := a.page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
				State:   playwright.LoadStateDomcontentloaded,
				Timeout: durationMS(defaultWait),
			}); err != nil {
				introErr = err
				break
			}
		}
		if introErr != nil {
			snapshot, snapshotErr := a.page.Evaluate(`() => ({
				url: window.location.href,
				hash: window.location.hash,
				browser: Boolean(document.querySelector('[data-testid="unixfs-browser"]')),
				visibleButtons: Array.from(document.querySelectorAll('button'))
					.filter((button) => button.getClientRects().length > 0)
					.map((button) => button.textContent?.trim())
					.filter(Boolean),
			})`)
			if snapshotErr == nil {
				return errors.Wrapf(introErr, "complete Drive intro (state: %v)", snapshot)
			}
			return errors.Wrap(introErr, "complete Drive intro")
		}
		if a.resetDriveOnReady {
			home := a.page.Locator("[role='button'][aria-label='Navigate to root']:visible, button[title='Home']:not([disabled]):visible").First()
			visible, err := home.IsVisible()
			if err != nil {
				return errors.Wrap(err, "inspect Drive home control")
			}
			if visible {
				if err := home.Click(); err != nil {
					return errors.Wrap(err, "return Drive to root")
				}
			}
			a.resetDriveOnReady = false
		}
	}
	selector := map[runtime.Event]string{
		runtime.EventAppReady:           "body",
		runtime.EventDriveReady:         "[data-testid='unixfs-browser']:visible",
		runtime.EventDriveSettled:       "[data-testid='unixfs-loading-diagnostics']",
		runtime.EventContentReady:       "pre",
		runtime.EventSpaceListConverged: "[data-testid='space-list'], [data-testid='resource-list'], body",
	}[event]
	if selector == "" {
		return errors.Errorf("unknown readiness event %q", event)
	}
	options := playwright.LocatorWaitForOptions{Timeout: durationMS(eventTimeout(event))}
	if event == runtime.EventDriveSettled {
		options.State = playwright.WaitForSelectorStateHidden
	}
	return a.page.Locator(selector).First().WaitFor(options)
}

func eventTimeout(event runtime.Event) time.Duration {
	switch event {
	case runtime.EventAppReady, runtime.EventDriveReady:
		return 2 * time.Minute
	case runtime.EventDriveSettled, runtime.EventContentReady, runtime.EventSpaceListConverged:
		return 30 * time.Second
	default:
		return defaultWait
	}
}

// OpenSecondTab opens route in a new page inside the current browser context.
func (a *Adapter) OpenSecondTab(route string) (runtime.Tab, error) {
	page, err := a.ctx.NewPage()
	if err != nil {
		return nil, err
	}
	old := a.page
	a.page = page
	if err := a.OpenRoute(route); err != nil {
		a.page = old
		_ = page.Close()
		return nil, err
	}
	a.page = old
	return &tab{page: page}, nil
}

// BackgroundTab reports that OS-level tab backgrounding is unsupported.
func (a *Adapter) BackgroundTab(tab runtime.Tab) error {
	return runtime.Unsupported("background-tab", "devwasm cannot control operating-system tab focus")
}

func isTransientNavigationError(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "execution context was destroyed") ||
		strings.Contains(message, "most likely because of a navigation")
}

// ReloadPage reloads the active browser page.
func (a *Adapter) ReloadPage() error {
	_, err := a.page.Reload()
	return err
}

// RestartWorkerHost reports that worker-host restart control is unsupported.
func (a *Adapter) RestartWorkerHost() error {
	return runtime.Unsupported("restart-worker-host", "devwasm does not expose worker-host lifecycle control")
}

type tab struct{ page playwright.Page }

func (t *tab) ID() string { return t.page.URL() }

func durationMS(d time.Duration) *float64 {
	ms := float64(d.Milliseconds())
	return &ms
}

var _ runtime.Runtime = (*Adapter)(nil)
