//go:build !js

// Package devwasm adapts the existing browser WASM harness to the scenario
// runtime contract.
package devwasm

import (
	"context"
	"fmt"
	"strings"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/s4wave/spacewave/e2e/runtime"
	"github.com/s4wave/spacewave/e2e/wasm"
	"github.com/sirupsen/logrus"
)

var eventTimeouts = map[runtime.Event]time.Duration{
	runtime.EventAppReady:           2 * time.Minute,
	runtime.EventDriveReady:         2 * time.Minute,
	runtime.EventDriveSettled:       30 * time.Second,
	runtime.EventContentReady:       30 * time.Second,
	runtime.EventSpaceListConverged: 30 * time.Second,
}

const defaultWait = 120 * time.Second

type Options struct{}

type Adapter struct {
	h                 *wasm.Harness
	ctx               playwright.BrowserContext
	page              playwright.Page
	resetDriveOnReady bool
}

func New(ctx context.Context, _ Options) (*Adapter, error) {
	compiler, err := wasm.ResolveE2EWasmCompiler()
	if err != nil {
		return nil, fmt.Errorf("resolve devwasm compiler: %w", err)
	}
	bootOptions := []wasm.Option{
		wasm.WithSessionHarness(),
		wasm.WithManifestBuildTimeout(20 * time.Minute),
	}
	switch compiler {
	case wasm.E2EWasmCompilerTinyGo:
		if err := wasm.ApplyE2EWasmTinyGoCompilerEnv(); err != nil {
			return nil, fmt.Errorf("configure TinyGo compiler: %w", err)
		}
		bootOptions = append(bootOptions, wasm.WithTinyGoCore())
	case wasm.E2EWasmCompilerGoScript:
		bootOptions = append(bootOptions, wasm.WithGoScriptBrowserStartup())
	}
	h, err := wasm.Boot(ctx, logrus.NewEntry(logrus.New()), bootOptions...)
	if err != nil {
		return nil, fmt.Errorf("boot devwasm harness: %w", err)
	}
	if err := h.LaunchBrowser(); err != nil {
		h.Release()
		return nil, fmt.Errorf("launch devwasm browser: %w", err)
	}
	a := &Adapter{h: h}
	if err := a.newPage(); err != nil {
		a.Close()
		return nil, err
	}
	return a, nil
}

func (a *Adapter) Name() string { return "devwasm" }

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
			return fmt.Errorf("create browser context: %w", err)
		}
	}
	a.page, err = a.ctx.NewPage()
	if err != nil {
		return fmt.Errorf("create browser page: %w", err)
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
				return fmt.Errorf("close session page: %w", err)
			}
			a.page = nil
		}
		return a.newPage()
	case runtime.SessionFreshInstall:
		if a.ctx != nil {
			if err := a.ctx.Close(); err != nil {
				return fmt.Errorf("close install context: %w", err)
			}
			a.ctx = nil
			a.page = nil
		}
		return a.newPage()
	default:
		return fmt.Errorf("unsupported session requirement %q", requirement)
	}
}

func (a *Adapter) OpenRoute(route string) error {
	if !strings.HasPrefix(route, "/") {
		route = "/" + route
	}
	if route == "/quickstart/drive" {
		a.resetDriveOnReady = true
		if !needsDocumentLoad(a.page.URL()) {
			home := a.page.Locator("[role='button'][aria-label='Navigate to root']:visible, button[title='Home']:not([disabled]):visible").First()
			visible, err := home.IsVisible()
			if err != nil {
				return fmt.Errorf("inspect Drive home control: %w", err)
			}
			if visible {
				if err := home.Click(); err != nil {
					return fmt.Errorf("return Drive to root before navigation: %w", err)
				}
				if _, err := a.page.Evaluate(`async () => {
					await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
				}`); err != nil {
					return fmt.Errorf("settle Drive root navigation: %w", err)
				}
			}
		}
	}
	if needsDocumentLoad(a.page.URL()) {
		if _, err := a.page.Goto(a.h.BaseURL() + "/#" + route); err != nil {
			return fmt.Errorf("open route %q: %w", route, err)
		}
	} else if err := a.openRouteClientSide("#" + route); err != nil {
		return fmt.Errorf("open route %q client-side: %w", route, err)
	}
	return a.WaitForEvent(runtime.EventAppReady)
}

func (a *Adapter) openRouteClientSide(targetHash string) error {
	_, err := a.page.Evaluate(`async ({ targetHash }) => {
		const markerKey = '__s4waveE2EOpenRouteMarker'
		const marker = String(Date.now()) + ':' + String(Math.random())
		window[markerKey] = marker
		await new Promise((resolve) => {
			const settle = () => {
				window.removeEventListener('hashchange', onHashChange)
				requestAnimationFrame(() => requestAnimationFrame(resolve))
			}
			const onHashChange = () => {
				if (window.location.hash === targetHash) settle()
			}
			if (window.location.hash === targetHash) {
				settle()
				return
			}
			window.addEventListener('hashchange', onHashChange)
			window.location.hash = targetHash
		})
		if (window[markerKey] !== marker) {
			throw new Error('warm navigation marker did not survive route change')
		}
		delete window[markerKey]
		return null
	}`, map[string]any{"targetHash": targetHash})
	return err
}

func needsDocumentLoad(pageURL string) bool {
	return pageURL == "" || pageURL == "about:blank"
}

func (a *Adapter) ClickControl(control string) error {
	if control == "confirm" {
		return a.page.Locator("input[placeholder='Folder name']:visible").First().Press("Enter")
	}
	if control == "new-folder" {
		input := a.page.Locator("input[placeholder='Folder name']:visible").First()
		buttons := a.page.Locator("button[title='New folder']:not([disabled]):visible")
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
		"new-folder":   "button[title='New folder']:not([disabled]):visible",
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
	return a.page.Locator(selector).Last().Click()
}

func (a *Adapter) DoubleClickContent(content string) error {
	return a.page.Locator("[role='row']:has-text('" + content + "'):visible").First().Dblclick()
}

func (a *Adapter) Type(field string, value string) error {
	selector := field
	if !strings.ContainsAny(field, "[]#.") {
		selector = "input[placeholder='" + field + "']"
	}
	return a.page.Locator(selector + ":visible").Last().Fill(value)
}

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

func (a *Adapter) MoveContent(source, target string) error {
	_, err := a.page.Evaluate(`({ source, target }) => {
		const row = Array.from(document.querySelectorAll('[role="row"]')).find((el) => el.textContent?.includes(target) && el.getClientRects().length > 0)
		if (!(row instanceof HTMLElement)) throw new Error('target row not found: ' + target)
		const transfer = new DataTransfer()
		transfer.setData('application/x-s4wave-app-drag+json', JSON.stringify({version: 1, items: [{id: source, label: source, capabilities: [{kind: 'movable', value: {case: 'unixfs-entry', value: {unixfsId: 'files', path: '/' + source, isDir: false}}}]}]}))
		for (const type of ['dragover', 'drop']) row.dispatchEvent(new DragEvent(type, {bubbles: true, cancelable: true, dataTransfer: transfer}))
	}`, map[string]any{"source": source, "target": target})
	return err
}

func (a *Adapter) ExpectVisible(content string) error {
	return a.waitForText(content, true)
}

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
func (a *Adapter) DeleteSpace() error {
	menu := a.page.Locator("[role='button'][aria-label='Open shared object menu']:visible").Last()
	if err := menu.Click(); err != nil {
		return fmt.Errorf("open shared object menu: %w", err)
	}
	danger := a.page.Locator("text=Danger Zone:visible").Last()
	if err := danger.Click(); err != nil {
		return fmt.Errorf("open danger zone: %w", err)
	}
	trigger := a.page.Locator("button:has-text('Delete Object'):visible").Last()
	if err := trigger.Click(); err != nil {
		return fmt.Errorf("open delete space dialog: %w", err)
	}
	input := a.page.Locator("[role='dialog'] input:visible").First()
	spaceName, err := input.GetAttribute("placeholder")
	if err != nil {
		return fmt.Errorf("read delete space confirmation name: %w", err)
	}
	if spaceName == "" {
		return fmt.Errorf("delete space confirmation has no space name")
	}
	if err := input.Fill(spaceName); err != nil {
		return fmt.Errorf("confirm delete space: %w", err)
	}
	if err := a.page.Locator("[role='dialog'] button:has-text('Delete Space'):visible").Last().Click(); err != nil {
		return fmt.Errorf("submit delete space: %w", err)
	}
	return a.ExpectRoute("/u/")
}

func (a *Adapter) ExpectRoute(route string) error {
	if !strings.Contains(a.page.URL(), route) {
		return fmt.Errorf("expected route %q, got %q", route, a.page.URL())
	}
	return nil
}

func (a *Adapter) WaitForEvent(event runtime.Event) error {
	if event == runtime.EventDriveReady {
		var introErr error
		for range 3 {
			_, introErr = a.page.Evaluate(`async () => {
				const deadline = Date.now() + 120000
				const labels = ['Next', 'Got it, start exploring', 'Open files']
				for (;;) {
					const action = Array.from(document.querySelectorAll('button')).find((button) => !button.disabled && labels.includes(button.textContent?.trim() ?? ''))
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
			return fmt.Errorf("complete Drive intro: %w", introErr)
		}
		if a.resetDriveOnReady {
			home := a.page.Locator("[role='button'][aria-label='Navigate to root']:visible, button[title='Home']:not([disabled]):visible").First()
			visible, err := home.IsVisible()
			if err != nil {
				return fmt.Errorf("inspect Drive home control: %w", err)
			}
			if visible {
				if err := home.Click(); err != nil {
					return fmt.Errorf("return Drive to root: %w", err)
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
		return fmt.Errorf("unknown readiness event %q", event)
	}
	timeout := eventTimeouts[event]
	if timeout == 0 {
		timeout = defaultWait
	}
	options := playwright.LocatorWaitForOptions{Timeout: durationMS(timeout)}
	if event == runtime.EventDriveSettled {
		options.State = playwright.WaitForSelectorStateHidden
	}
	return a.page.Locator(selector).First().WaitFor(options)
}

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

func (a *Adapter) BackgroundTab(tab runtime.Tab) error {
	return runtime.Unsupported("background-tab", "devwasm cannot control operating-system tab focus")
}
func isTransientNavigationError(message string) bool {
	message = strings.ToLower(message)
	return strings.Contains(message, "execution context was destroyed") ||
		strings.Contains(message, "most likely because of a navigation")
}

func (a *Adapter) ReloadPage() error {
	_, err := a.page.Reload()
	return err
}

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
