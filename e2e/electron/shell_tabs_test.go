//go:build !skip_e2e && !js

package electron

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	s4wave_layout "github.com/s4wave/spacewave/sdk/layout"
)

const shellUIWaitTimeout = 120_000

// TIER: nightly
func TestShellTabSelectionPersistsInWindowSession(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	mainPage, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	closeOtherAppPages(t, h, mainPage)
	if err := seedShellTabs(mainPage); err != nil {
		t.Fatal(err)
	}
	if err := waitForSelectedShellTab(mainPage, "Home"); err != nil {
		t.Fatal(err)
	}
	if err := waitForStoredActiveTabID(mainPage, "home"); err != nil {
		t.Fatal(err)
	}

	if err := clickShellTab(mainPage, "Changelog"); err != nil {
		t.Fatal(err)
	}
	if err := waitForSelectedShellTab(mainPage, "Changelog"); err != nil {
		t.Fatal(err)
	}
	if err := waitForStoredActiveTabID(mainPage, "changelog"); err != nil {
		t.Fatal(err)
	}

	if err := clickShellTab(mainPage, "Home"); err != nil {
		t.Fatal(err)
	}
	if err := waitForSelectedShellTab(mainPage, "Home"); err != nil {
		t.Fatal(err)
	}
	if err := waitForStoredActiveTabID(mainPage, "home"); err != nil {
		t.Fatal(err)
	}
}

// TIER: nightly
func TestShellTabsSurviveRendererReload(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	page, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	closeOtherAppPages(t, h, page)
	if err := seedShellTabs(page); err != nil {
		t.Fatal(err)
	}
	if err := waitForShellTab(page, "Changelog"); err != nil {
		t.Fatal(err)
	}

	if _, err := page.Reload(); err != nil {
		t.Fatal(err)
	}
	page, err = waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitForShellTab(page, "Docs"); err != nil {
		t.Fatal(err)
	}
	if err := waitForShellTab(page, "Changelog"); err != nil {
		t.Fatal(err)
	}
}

// TIER: nightly
func TestHelpDocumentationSelectsDocsShellTab(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Minute)
	defer cancel()
	page, err := waitForShellPage(ctx, h)
	if err != nil {
		t.Fatal(err)
	}
	closeOtherAppPages(t, h, page)
	if err := page.SetViewportSize(1440, 900); err != nil {
		t.Fatalf("set viewport size: %v", err)
	}
	if err := seedGridShellTabs(t, page); err != nil {
		t.Fatal(err)
	}
	if err := waitForSelectedShellTab(page, "Home"); err != nil {
		t.Fatal(err)
	}

	if err := invokeHelpDocumentationMenu(page); err != nil {
		t.Fatal(err)
	}

	if err := waitForSelectedShellTab(page, "Docs"); err != nil {
		t.Fatal(err)
	}
	if err := waitForStoredActiveShellTab(page, "Docs", "/docs"); err != nil {
		t.Fatal(err)
	}
	if err := waitForDocsHeading(page); err != nil {
		t.Fatal(err)
	}
}

func waitForShellPage(ctx context.Context, h *Harness) (playwright.Page, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, page := range h.AppPages() {
			hasShell, err := pageHasShellTabs(page)
			if err == nil && hasShell {
				return page, nil
			}
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-h.done:
			return nil, h.desktopRuntimeErr("desktop runtime exited before shell page appeared")
		case <-ticker.C:
		}
	}
}

func closeOtherAppPages(t *testing.T, h *Harness, keep playwright.Page) {
	t.Helper()

	for _, page := range h.AppPages() {
		if page == keep {
			continue
		}
		closePageIfOpen(t, page)
	}
}

func closePageIfOpen(t *testing.T, page playwright.Page) {
	t.Helper()

	if page.IsClosed() {
		return
	}
	if err := page.Close(); err != nil {
		t.Fatalf("close app page: %v", err)
	}
}

func pageHasShellTabs(page playwright.Page) (bool, error) {
	raw, err := page.Evaluate(
		`() => document.querySelectorAll('.shell-flexlayout .flexlayout__tab_button').length > 0`,
	)
	if err != nil {
		return false, err
	}
	hasShell, _ := raw.(bool)
	return hasShell, nil
}

// seedShellTabs installs a known tab set and selection, then reloads so the
// Shell adopts them.
//
// The tab records live in the browser Shell Tabs store and the document-local
// selection sits beside them in session storage. Reloading is what makes the
// selection stick: the Shell honours a persisted selection only when the
// navigation continues the same document, and a fresh load starts its own
// incarnation instead.
func seedShellTabs(page playwright.Page) error {
	if _, err := page.Evaluate(`() => {
		const records = [
			{ id: 'home', name: 'Home', path: '/', creationSequence: 1 },
			{ id: 'docs', name: 'Docs', path: '/docs', creationSequence: 2 },
			{ id: 'changelog', name: 'Changelog', path: '/changelog', creationSequence: 3 },
		]
		localStorage.setItem(
			'browser-shell-tabs',
			JSON.stringify({ schemaVersion: 1, epoch: 1, revision: 1, records }),
		)
		sessionStorage.setItem(
			'shell-document-state',
			JSON.stringify({ incarnation: 'e2e-shell-tabs', activeTabId: 'home' }),
		)
		sessionStorage.removeItem('shell-tabs-layout')
	}`); err != nil {
		return err
	}
	if _, err := page.Reload(); err != nil {
		return err
	}
	// Wait for the seeded names rather than a tab count. The Shell opens a
	// default tab of its own, so a count is satisfied whether or not the seed
	// reached the store, and every later step then fails somewhere unrelated.
	_, err := page.WaitForFunction(
		`() => {
			const names = Array.from(
				document.querySelectorAll('.shell-flexlayout .flexlayout__tab_button_content'),
			).map((content) => content.textContent?.trim())
			return ['Home', 'Docs', 'Changelog'].every((name) => names.includes(name))
		}`,
		nil,
		playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(shellUIWaitTimeout),
		},
	)
	return err
}

func seedGridShellTabs(t testing.TB, page playwright.Page) error {
	t.Helper()

	layoutData := encodeShellGridLayout(t)
	if _, err := page.Evaluate(`(layoutData) => {
		const records = [
			{ id: 'grid-home', name: 'Home', path: '/', creationSequence: 1 },
			{ id: 'grid-blog', name: 'Blog', path: '/blog', creationSequence: 2 },
		]
		localStorage.setItem(
			'browser-shell-tabs',
			JSON.stringify({ schemaVersion: 1, epoch: 1, revision: 1, records }),
		)
		sessionStorage.setItem(
			'shell-document-state',
			JSON.stringify({ incarnation: 'e2e-shell-grid', activeTabId: 'grid-home' }),
		)
		sessionStorage.removeItem('shell-tabs-layout')
		window.location.hash = '#/g/' + layoutData
	}`, layoutData); err != nil {
		return err
	}
	if _, err := page.Reload(); err != nil {
		return err
	}
	_, err := page.WaitForFunction(
		`() => {
			if (!window.location.hash.startsWith('#/g/')) return false
			const names = Array.from(
				document.querySelectorAll('.shell-flexlayout .flexlayout__tab_button_content'),
			).map((content) => content.textContent?.trim())
			return ['Home', 'Blog'].every((name) => names.includes(name))
		}`,
		nil,
		playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(shellUIWaitTimeout),
		},
	)
	return err
}

func encodeShellGridLayout(t testing.TB) string {
	t.Helper()

	snapshot := &s4wave_layout.LayoutSnapshot{
		Model: &s4wave_layout.LayoutModel{
			Layout: &s4wave_layout.RowDef{
				Weight: 100,
				Children: []*s4wave_layout.RowOrTabSetDef{
					shellGridTabset("grid-left", "grid-home", "Home"),
					shellGridTabset("grid-right", "grid-blog", "Blog"),
				},
			},
		},
		LocalState: &s4wave_layout.LayoutLocalState{
			ActiveTabSetId: "grid-left",
			TabSetSelections: map[string]string{
				"grid-left":  "grid-home",
				"grid-right": "grid-blog",
			},
		},
	}
	data, err := snapshot.MarshalVT()
	if err != nil {
		t.Fatalf("marshal shell grid layout: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func shellGridTabset(tabsetID, tabID, name string) *s4wave_layout.RowOrTabSetDef {
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

func invokeHelpDocumentationMenu(page playwright.Page) error {
	clickOptions := playwright.LocatorClickOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	}
	waitOptions := playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	}

	if err := page.Locator("button:visible:has-text('Help')").First().Click(clickOptions); err != nil {
		return fmt.Errorf("click Help menu: %w; state=%s", err, shellMenuDebug(page))
	}
	documentationItem := page.Locator("[role='menuitem']:visible:has-text('Documentation')").First()
	if err := documentationItem.WaitFor(waitOptions); err != nil {
		return fmt.Errorf("wait for Documentation menu item: %w; state=%s", err, shellMenuDebug(page))
	}
	if err := documentationItem.Click(clickOptions); err != nil {
		return fmt.Errorf("click Documentation menu item: %w; state=%s", err, shellMenuDebug(page))
	}
	return nil
}

func shellMenuDebug(page playwright.Page) string {
	raw, err := page.Evaluate(`() => {
		const compactRect = (element) => {
			const rect = element.getBoundingClientRect()
			return {
				x: Math.round(rect.x),
				y: Math.round(rect.y),
				width: Math.round(rect.width),
				height: Math.round(rect.height),
			}
		}
		return JSON.stringify({
			hash: window.location.hash,
			buttons: Array.from(document.querySelectorAll('button')).map((button) => ({
				text: button.textContent?.trim() ?? '',
				ariaExpanded: button.getAttribute('aria-expanded'),
				state: button.getAttribute('data-state'),
				rect: compactRect(button),
			})),
			menuitems: Array.from(document.querySelectorAll('[role="menuitem"]')).map((item) => ({
				text: item.textContent?.trim() ?? '',
				state: item.getAttribute('data-state'),
				rect: compactRect(item),
			})),
			shellDocumentState: sessionStorage.getItem('shell-document-state'),
			shellTabRecords: localStorage.getItem('browser-shell-tabs'),
		})
	}`)
	if err != nil {
		return fmt.Sprintf("debug unavailable: %v", err)
	}
	text, _ := raw.(string)
	return text
}

func clickShellTab(page playwright.Page, name string) error {
	_, err := page.Evaluate(`(name) => {
		const buttons = Array.from(
			document.querySelectorAll('.shell-flexlayout .flexlayout__tab_button'),
		)
		const button = buttons.find((candidate) => {
			const content = candidate.querySelector('.flexlayout__tab_button_content')
			return content?.textContent?.trim() === name
		})
		if (!button) {
			throw new Error('shell tab not found: ' + name)
		}
		button.click()
	}`, name)
	return err
}

func waitForShellTab(page playwright.Page, name string) error {
	_, err := page.WaitForFunction(`(name) => {
		const buttons = Array.from(
			document.querySelectorAll('.shell-flexlayout .flexlayout__tab_button'),
		)
		return buttons.some((candidate) => {
			const content = candidate.querySelector('.flexlayout__tab_button_content')
			return content?.textContent?.trim() === name
		})
	}`, name, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	})
	return err
}

func waitForSelectedShellTab(page playwright.Page, name string) error {
	_, err := page.WaitForFunction(`(name) => {
		const content = document.querySelector(
			'.shell-flexlayout .flexlayout__tab_button--selected .flexlayout__tab_button_content',
		)
		return content?.textContent?.trim() === name
	}`, name, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	})
	return err
}

func waitForStoredActiveTabID(page playwright.Page, tabID string) error {
	_, err := page.WaitForFunction(`(tabID) => {
		try {
			const documentState = JSON.parse(sessionStorage.getItem('shell-document-state') ?? 'null')
			return documentState?.activeTabId === tabID
		} catch {
			return false
		}
	}`, tabID, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	})
	return err
}

// waitForStoredActiveShellTab resolves the persisted selection against the tab
// records, since the document holds only the selected id and the records hold
// the name and path it refers to.
func waitForStoredActiveShellTab(page playwright.Page, name, path string) error {
	_, err := page.WaitForFunction(`([name, path]) => {
		try {
			const documentState = JSON.parse(sessionStorage.getItem('shell-document-state') ?? 'null')
			const snapshot = JSON.parse(localStorage.getItem('browser-shell-tabs') ?? 'null')
			const active = snapshot?.records?.find((record) => record.id === documentState?.activeTabId)
			return active?.name === name && active?.path === path
		} catch {
			return false
		}
	}`, []string{name, path}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	})
	return err
}

func waitForDocsHeading(page playwright.Page) error {
	_, err := page.WaitForFunction(`() => {
		return Array.from(document.querySelectorAll('h1, h2')).some(
			(candidate) => candidate.textContent?.trim() === 'Documentation',
		)
	}`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	})
	return err
}
