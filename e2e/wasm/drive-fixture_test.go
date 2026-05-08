//go:build !skip_e2e && !js

package wasm

import (
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

func createDriveFolder(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	waitForDriveSettled(t, page)
	if err := page.Locator("button[title='New folder']:not([disabled])").First().Click(); err != nil {
		t.Fatalf("click new folder: %v", err)
	}
	input := page.Locator("input[placeholder='Folder name']").First()
	if err := input.WaitFor(); err != nil {
		t.Fatalf("wait for new folder input %q: %v", name, err)
	}
	if err := input.Fill(name); err != nil {
		t.Fatalf("fill new folder name %q: %v", name, err)
	}
	if err := input.Press("Enter"); err != nil {
		t.Fatalf("confirm new folder %q: %v", name, err)
	}
	waitForDriveEntry(t, page, name)
}

func waitForDriveSettled(t testing.TB, page playwright.Page) {
	t.Helper()

	diagnostics := page.Locator("[data-testid='unixfs-loading-diagnostics']")
	if err := diagnostics.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateHidden,
	}); err != nil {
		t.Fatalf("wait for drive loading state to clear: %v", err)
	}
}
