//go:build !skip_e2e && !js

package electron

import (
	"context"
	"fmt"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
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

func seedShellTabs(page playwright.Page) error {
	if _, err := page.Evaluate(`() => {
		const tabs = [
			{ id: 'home', name: 'Home', path: '/' },
			{ id: 'docs', name: 'Docs', path: '/docs' },
			{ id: 'changelog', name: 'Changelog', path: '/changelog' },
		]
		const nextState = JSON.stringify({ tabs, activeTabId: 'home' })
		sessionStorage.setItem(
			'shell-tabs-state',
			nextState,
		)
		sessionStorage.removeItem('shell-tabs-layout')
	}`); err != nil {
		return err
	}
	if _, err := page.Reload(); err != nil {
		return err
	}
	_, err := page.WaitForFunction(
		`() => document.querySelectorAll('.shell-flexlayout .flexlayout__tab_button').length >= 3`,
		nil,
		playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(shellUIWaitTimeout),
		},
	)
	return err
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

func assertSelectedShellTab(page playwright.Page, want string) error {
	got, err := selectedShellTab(page)
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("expected selected shell tab %q, got %q", want, got)
	}
	return nil
}

func selectedShellTab(page playwright.Page) (string, error) {
	raw, err := page.Evaluate(`() => {
		const content = document.querySelector(
			'.shell-flexlayout .flexlayout__tab_button--selected .flexlayout__tab_button_content',
		)
		return content?.textContent?.trim() ?? ''
	}`)
	if err != nil {
		return "", err
	}
	text, _ := raw.(string)
	return text, nil
}

func waitForStoredActiveTabID(page playwright.Page, tabID string) error {
	_, err := page.WaitForFunction(`(tabID) => {
		const raw = sessionStorage.getItem('shell-tabs-state')
		if (!raw) return false
		return JSON.parse(raw).activeTabId === tabID
	}`, tabID, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	})
	return err
}
