//go:build !skip_e2e && !js

package wasm

import (
	"encoding/base64"
	"fmt"
	"strings"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
)

const (
	splitDriveLeftTabID    = "tinygo-drive-left"
	splitDriveRightTabID   = "tinygo-drive-right"
	splitDriveLeftTabset   = "tinygo-drive-left-tabset"
	splitDriveRightTabset  = "tinygo-drive-right-tabset"
	splitDriveUploadName   = "tinygo-split-upload.md"
	splitDriveUploadNeedle = "TinyGo split-tab uploaded file stays visible in the right pane."
	splitDriveWaitTimeout  = 60 * time.Second
)

const splitDriveUploadBody = `# TinyGo split tab upload

TinyGo split-tab uploaded file stays visible in the right pane.

Navigation churn should not stall root readdir.
`

// TestQuickstartDriveSplitTabNavigation reproduces the staging TinyGo Drive
// split-tab path: create a Drive, upload a file, view two file tabs side by
// side, and churn file browser navigation while collecting stuck-state
// diagnostics on failures.
func TestQuickstartDriveSplitTabNavigation(t *testing.T) {
	sess := testHarness.NewCleanSession(t)
	page := sess.Page()
	if err := page.SetViewportSize(1440, 900); err != nil {
		t.Fatalf("set viewport size: %v", err)
	}
	console, stopConsole := sess.WatchConsole()
	defer stopConsole()

	probe := newSplitDriveProbe(t, page, console)
	scenario := CreateDriveScenario(t, testHarness, sess)
	page = scenario.GetSession().Page()
	probe.page = page
	WaitForDriveReady(t, testHarness, page)

	upload := playwright.InputFile{
		Name:     splitDriveUploadName,
		MimeType: "text/markdown",
		Buffer:   []byte(splitDriveUploadBody),
	}
	UploadViaPicker(t, page, []playwright.InputFile{upload})
	probe.waitForPaneEntry(0, splitDriveUploadName)
	verifyUploadedFile(t, scenario, page, upload)

	probe.openPaneEntry(0, splitDriveUploadName)
	probe.waitForPaneText(0, "uploaded file before split", splitDriveUploadNeedle)
	uploadedPath := currentSplitDriveAppPath(t, page)

	probe.clickPaneButton(0, "Up")
	probe.waitForPaneEntry(0, gettingStartedFileName)
	probe.openPaneEntry(0, gettingStartedFileName)
	probe.waitForPaneText(0, "getting-started before split", gettingStartedWelcomeText)
	gettingStartedPath := currentSplitDriveAppPath(t, page)

	probe.enterSplitLayout(gettingStartedPath, uploadedPath)
	probe.waitForPaneText(0, "left split getting-started", gettingStartedWelcomeText)
	probe.waitForPaneText(1, "right split uploaded file", splitDriveUploadNeedle)

	probe.clickPaneButton(1, "Up")
	probe.waitForPaneEntry(1, splitDriveUploadName)
	probe.openPaneEntry(1, splitDriveUploadName)
	probe.waitForPaneText(1, "right split uploaded file after reopen", splitDriveUploadNeedle)

	probe.clickPaneButton(0, "Up")
	probe.waitForPaneEntry(0, gettingStartedFileName)
	probe.openPaneEntry(0, gettingStartedFileName)
	probe.waitForPaneText(0, "left split getting-started after reopen", gettingStartedWelcomeText)

	probe.clickPaneButton(1, "Back")
	probe.waitForPaneEntry(1, splitDriveUploadName)
	probe.clickPaneButton(1, "Forward")
	probe.waitForPaneText(1, "right split uploaded file after forward", splitDriveUploadNeedle)

	report := DrainCrashReport(console)
	if report.HasCrash() {
		t.Fatalf("unexpected browser/WASM crash report after split-tab Drive navigation: %+v\n%s", report, probe.captureDiagnostics())
	}
	if report.HasExitedGoLoop() {
		t.Fatalf("unexpected exited-Go loop after split-tab Drive navigation: %+v\n%s", report, probe.captureDiagnostics())
	}
}

type splitDriveProbe struct {
	t       testing.TB
	page    playwright.Page
	console <-chan string
}

func newSplitDriveProbe(t testing.TB, page playwright.Page, console <-chan string) *splitDriveProbe {
	t.Helper()

	return &splitDriveProbe{
		t:       t,
		page:    page,
		console: console,
	}
}

func (p *splitDriveProbe) enterSplitLayout(leftPath, rightPath string) {
	p.t.Helper()

	layoutData := encodeSplitDriveLayout(p.t)
	_, err := p.page.Evaluate(`({ layoutData, leftPath, rightPath }) => {
		const tabs = [
			{
				id: 'tinygo-drive-left',
				name: 'getting-started.md',
				path: leftPath,
				customName: 'getting-started.md',
			},
			{
				id: 'tinygo-drive-right',
				name: 'tinygo-split-upload.md',
				path: rightPath,
				customName: 'tinygo-split-upload.md',
			},
		]
		sessionStorage.setItem(
			'shell-tabs-state',
			JSON.stringify({ tabs, activeTabId: 'tinygo-drive-right' }),
		)
		sessionStorage.removeItem('shell-tabs-layout')
		window.location.hash = '#/g/' + layoutData
	}`, map[string]any{
		"layoutData": layoutData,
		"leftPath":   leftPath,
		"rightPath":  rightPath,
	})
	if err != nil {
		p.fail("seed split shell tabs", err)
	}
	if _, err := p.page.Reload(); err != nil {
		p.fail("reload split shell route", err)
	}
	WaitForApp(p.t, p.page)
	p.waitForShellGrid()
}

func (p *splitDriveProbe) waitForShellGrid() {
	p.t.Helper()

	_, err := p.page.WaitForFunction(`() => {
		if (!window.location.hash.startsWith('#/g/')) return false
		const panes = Array.from(
			document.querySelectorAll('.shell-flexlayout .flexlayout__tabset'),
		)
		const visiblePanes = panes
			.map((pane) => {
				const rect = pane.getBoundingClientRect()
				return { pane, rect, browser: pane.querySelector('[data-testid="unixfs-browser"]') }
			})
			.filter(({ rect, browser }) => browser && rect.width > 0 && rect.height > 0)
		return (
			visiblePanes.length >= 2 &&
			visiblePanes[0].rect.left < visiblePanes[1].rect.left
		)
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: splitDriveTimeoutMS(),
	})
	if err != nil {
		p.fail("wait for vertical split shell grid", err)
	}
	p.waitForPaneSettled(0, "left split initial load")
	p.waitForPaneSettled(1, "right split initial load")
}

func (p *splitDriveProbe) waitForPaneText(pane int, label, want string) {
	p.t.Helper()

	_, err := p.page.WaitForFunction(`({ pane, want }) => {
		const paneNode = document.querySelectorAll(
			'.shell-flexlayout .flexlayout__tabset',
		)[pane]
		const browser = paneNode?.querySelector('[data-testid="unixfs-browser"]')
		const pre = browser?.querySelector('pre')
		return Boolean(pre?.textContent?.includes(want))
	}`, map[string]any{"pane": pane, "want": want}, playwright.PageWaitForFunctionOptions{
		Timeout: splitDriveTimeoutMS(),
	})
	if err != nil {
		p.fail("wait for "+label, err)
	}
	text, err := p.paneBrowser(pane).Locator("pre").First().TextContent()
	if err != nil {
		p.fail("read "+label, err)
	}
	if !strings.Contains(text, want) {
		p.fail(
			fmt.Sprintf("expected %s to include %q, got %q", label, want, strings.TrimSpace(text)),
			nil,
		)
	}
	p.waitForPaneSettled(pane, label)
}

func (p *splitDriveProbe) waitForPaneEntry(pane int, name string) {
	p.t.Helper()

	row := p.paneBrowser(pane).Locator("[role='row']").Locator("text=" + name).First()
	if err := row.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: splitDriveTimeoutMS(),
	}); err != nil {
		p.fail(fmt.Sprintf("wait for pane %d drive entry %s", pane, name), err)
	}
	p.waitForPaneSettled(pane, "pane entry "+name)
}

func (p *splitDriveProbe) openPaneEntry(pane int, name string) {
	p.t.Helper()

	p.waitForPaneEntry(pane, name)
	row := p.paneBrowser(pane).Locator("[role='row']").Locator("text=" + name).First()
	if err := row.Dblclick(); err != nil {
		p.fail(fmt.Sprintf("open pane %d drive entry %s", pane, name), err)
	}
}

func (p *splitDriveProbe) clickPaneButton(pane int, title string) {
	p.t.Helper()

	button := p.paneBrowser(pane).Locator("button[title='" + title + "']:not([disabled])").First()
	if err := button.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: splitDriveTimeoutMS(),
	}); err != nil {
		p.fail(fmt.Sprintf("wait for pane %d %s button", pane, title), err)
	}
	if err := button.Click(); err != nil {
		p.fail(fmt.Sprintf("click pane %d %s button", pane, title), err)
	}
	p.waitForPaneSettled(pane, fmt.Sprintf("pane %d after %s", pane, title))
}

func (p *splitDriveProbe) waitForPaneSettled(pane int, label string) {
	p.t.Helper()

	diagnostics := p.paneBrowser(pane).Locator("[data-testid='unixfs-loading-diagnostics']")
	if err := diagnostics.WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateHidden,
		Timeout: splitDriveTimeoutMS(),
	}); err != nil {
		p.fail("wait for "+label+" loading diagnostics to clear", err)
	}
}

func (p *splitDriveProbe) paneBrowser(pane int) playwright.Locator {
	p.t.Helper()

	return p.page.Locator(".shell-flexlayout .flexlayout__tabset").Nth(pane).Locator("[data-testid='unixfs-browser']").First()
}

func (p *splitDriveProbe) fail(label string, err error) {
	p.t.Helper()

	errText := "<nil>"
	if err != nil {
		errText = err.Error()
	}
	report := DrainCrashReport(p.console)
	p.t.Fatalf(
		"%s: %s\ncrash report: %+v\n%s",
		label,
		errText,
		report,
		p.captureDiagnostics(),
	)
}

func (p *splitDriveProbe) captureDiagnostics() string {
	raw, err := p.page.Evaluate(`() => {
		const cleanText = (el) => (el?.textContent || '')
			.replace(/\s+/g, ' ')
			.trim()
			.slice(0, 2000)
		const rectOf = (el) => {
			const rect = el.getBoundingClientRect()
			return {
				x: Math.round(rect.x),
				y: Math.round(rect.y),
				width: Math.round(rect.width),
				height: Math.round(rect.height),
			}
		}
		const readButton = (button) => ({
			title: button.getAttribute('title') || '',
			label: cleanText(button),
			disabled: button.disabled,
		})
		const readBrowser = (browser, index) => ({
			index,
			rect: rectOf(browser),
			text: cleanText(browser),
			rows: Array.from(browser.querySelectorAll('[role="row"]'), cleanText).slice(0, 30),
			buttons: Array.from(browser.querySelectorAll('button'), readButton),
			loadingDiagnostics: Array.from(
				browser.querySelectorAll('[data-testid="unixfs-loading-diagnostics"]'),
				cleanText,
			),
		})
		const readPane = (pane, index) => ({
			index,
			rect: rectOf(pane),
			text: cleanText(pane),
			browsers: Array.from(
				pane.querySelectorAll('[data-testid="unixfs-browser"]'),
				readBrowser,
			),
		})
		return JSON.stringify({
			url: window.location.href,
			hash: window.location.hash,
			gridRoute: window.location.hash.startsWith('#/g/'),
			body: cleanText(document.body),
			panes: Array.from(
				document.querySelectorAll('.shell-flexlayout .flexlayout__tabset'),
				readPane,
			),
			loadingDiagnostics: Array.from(
				document.querySelectorAll('[data-testid="unixfs-loading-diagnostics"]'),
				cleanText,
			),
			watchReaddirMarkers: globalThis.__s4wave_debug?.unixfs?.watchReaddir ?? null,
		}, null, 2)
	}`)
	if err != nil {
		return "split drive diagnostics unavailable: " + err.Error()
	}
	text, ok := raw.(string)
	if !ok {
		return fmt.Sprintf("split drive diagnostics: %#v", raw)
	}
	return "split drive diagnostics:\n" + text
}

func currentSplitDriveAppPath(t testing.TB, page playwright.Page) string {
	t.Helper()

	raw, err := page.Evaluate(`() => {
		const hash = window.location.hash || '#/'
		if (!hash.startsWith('#')) return '/'
		const path = hash.slice(1) || '/'
		return path.startsWith('/') ? path : '/' + path
	}`)
	if err != nil {
		t.Fatalf("read app path: %v", err)
	}
	path, ok := raw.(string)
	if !ok || path == "" {
		t.Fatalf("read app path returned %#v", raw)
	}
	if !strings.HasPrefix(path, "/") {
		t.Fatalf("expected app path to start with /, got %q", path)
	}
	return path
}

func encodeSplitDriveLayout(t testing.TB) string {
	t.Helper()

	snapshot := &s4wave_layout.LayoutSnapshot{
		Model: &s4wave_layout.LayoutModel{
			Layout: &s4wave_layout.RowDef{
				Weight: 100,
				Children: []*s4wave_layout.RowOrTabSetDef{
					splitDriveTabset(splitDriveLeftTabset, splitDriveLeftTabID, gettingStartedFileName),
					splitDriveTabset(splitDriveRightTabset, splitDriveRightTabID, splitDriveUploadName),
				},
			},
		},
		LocalState: &s4wave_layout.LayoutLocalState{
			ActiveTabSetId: splitDriveRightTabset,
			TabSetSelections: map[string]string{
				splitDriveLeftTabset:  splitDriveLeftTabID,
				splitDriveRightTabset: splitDriveRightTabID,
			},
		},
	}
	data, err := snapshot.MarshalVT()
	if err != nil {
		t.Fatalf("marshal split drive layout: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func splitDriveTabset(tabsetID, tabID, name string) *s4wave_layout.RowOrTabSetDef {
	return &s4wave_layout.RowOrTabSetDef{
		Node: &s4wave_layout.RowOrTabSetDef_TabSet{
			TabSet: &s4wave_layout.TabSetDef{
				Id:     tabsetID,
				Weight: 50,
				Children: []*s4wave_layout.TabDef{
					{
						Id:   tabID,
						Name: name,
					},
				},
			},
		},
	}
}

func splitDriveTimeoutMS() *float64 {
	return playwright.Float(float64(splitDriveWaitTimeout / time.Millisecond))
}
