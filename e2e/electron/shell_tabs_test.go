//go:build !skip_e2e && !js

package electron

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	playwright "github.com/playwright-community/playwright-go"
)

const shellUIWaitTimeout = 120_000

// TIER: nightly
func TestShellTabSelectionIsWindowLocal(t *testing.T) {
	h := testHarness
	if h == nil {
		t.Fatal("expected electron harness")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	mainPage, err := h.WaitForPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := seedShellTabs(mainPage); err != nil {
		t.Fatal(err)
	}
	if err := waitForSelectedShellTab(mainPage, "Home"); err != nil {
		t.Fatal(err)
	}

	if _, err := mainPage.Evaluate(`() => {
		const baseUrl = location.protocol + '//' + location.host + location.pathname
		window.open(baseUrl + '#/docs', '_blank', 'noopener,noreferrer')
	}`); err != nil {
		t.Fatal(err)
	}

	pages, err := h.WaitForAppPages(ctx, 2)
	if err != nil {
		t.Fatal(err)
	}
	popoutPage := findOtherPageWithHash(pages, mainPage, "#/docs")
	if popoutPage == nil {
		t.Fatalf("expected popout page at #/docs, got %v", appPageURLs(pages))
	}
	if err := waitForSelectedShellTab(popoutPage, "Docs"); err != nil {
		t.Fatal(err)
	}
	if err := waitForStoredActiveTabID(mainPage, "docs"); err != nil {
		t.Fatal(err)
	}
	if err := assertSelectedShellTab(mainPage, "Home"); err != nil {
		t.Fatal(err)
	}

	if err := clickShellTab(mainPage, "Changelog"); err != nil {
		t.Fatal(err)
	}
	if err := waitForSelectedShellTab(mainPage, "Changelog"); err != nil {
		t.Fatal(err)
	}
	if err := waitForStoredActiveTabID(popoutPage, "changelog"); err != nil {
		t.Fatal(err)
	}
	if err := assertSelectedShellTab(popoutPage, "Docs"); err != nil {
		t.Fatal(err)
	}

	if err := clickShellTab(popoutPage, "Home"); err != nil {
		t.Fatal(err)
	}
	if err := waitForSelectedShellTab(popoutPage, "Home"); err != nil {
		t.Fatal(err)
	}
	if err := waitForStoredActiveTabID(mainPage, "home"); err != nil {
		t.Fatal(err)
	}
	if err := assertSelectedShellTab(mainPage, "Changelog"); err != nil {
		t.Fatal(err)
	}
}

func seedShellTabs(page playwright.Page) error {
	if _, err := page.Evaluate(`() => {
		const tabs = [
			{ id: 'home', name: 'Home', path: '/' },
			{ id: 'docs', name: 'Docs', path: '/docs' },
			{ id: 'changelog', name: 'Changelog', path: '/changelog' },
		]
		localStorage.setItem(
			'shell-tabs-state',
			JSON.stringify({ tabs, activeTabId: 'home' }),
		)
		localStorage.removeItem('shell-tabs-layout')
		window.location.hash = '#/'
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
		const raw = localStorage.getItem('shell-tabs-state')
		if (!raw) return false
		return JSON.parse(raw).activeTabId === tabID
	}`, tabID, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(shellUIWaitTimeout),
	})
	return err
}

func findOtherPageWithHash(
	pages []playwright.Page,
	current playwright.Page,
	hash string,
) playwright.Page {
	for _, page := range pages {
		if page == current {
			continue
		}
		if strings.Contains(page.URL(), hash) {
			return page
		}
	}
	return nil
}

func appPageURLs(pages []playwright.Page) []string {
	urls := make([]string, 0, len(pages))
	for _, page := range pages {
		urls = append(urls, page.URL())
	}
	return urls
}
