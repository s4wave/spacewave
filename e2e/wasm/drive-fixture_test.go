//go:build !skip_e2e && !js

package wasm

import (
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

const (
	gettingStartedFileName    = "getting-started.md"
	gettingStartedWelcomeText = "Welcome to your new drive"
	driveWelcomeSelector      = "[data-testid='drive-welcome']"
	driveInviteCTASelector    = "[data-testid='drive-invite-cta']"
)

func createDriveFolder(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	browser := visibleDriveBrowser(page)
	waitForDriveSettled(t, page)
	if err := browser.Locator("button[title='New folder']:not([disabled])").Click(); err != nil {
		t.Fatalf("click new folder: %v", err)
	}
	input := browser.Locator("input[placeholder='Folder name']")
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

	diagnostics := visibleDriveBrowser(page).Locator("[data-testid='unixfs-loading-diagnostics']")
	if err := diagnostics.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateHidden,
	}); err != nil {
		t.Fatalf("wait for drive loading state to clear: %v", err)
	}
}

func openGettingStartedFile(t testing.TB, page playwright.Page) {
	t.Helper()

	waitForDriveEntry(t, page, gettingStartedFileName)
	row := visibleDriveBrowser(page).Locator("[role='row']:has-text('" + gettingStartedFileName + "')").First()
	if err := row.Dblclick(); err != nil {
		t.Fatalf("open %s row: %v", gettingStartedFileName, err)
	}
	waitForGettingStartedContentView(t, page)
}

func waitForGettingStartedContentView(t testing.TB, page playwright.Page) {
	t.Helper()

	content := visibleDriveBrowser(page).Locator("pre").First()
	if err := content.WaitFor(); err != nil {
		t.Fatalf("wait for %s content view: %v", gettingStartedFileName, err)
	}
	text, err := content.TextContent()
	if err != nil {
		t.Fatalf("read %s content view: %v", gettingStartedFileName, err)
	}
	if !strings.Contains(text, gettingStartedWelcomeText) {
		t.Fatalf(
			"expected %s content view to include %q, got %q",
			gettingStartedFileName,
			gettingStartedWelcomeText,
			strings.TrimSpace(text),
		)
	}
}

func assertDriveRoute(t testing.TB, page playwright.Page, sessionIndex uint32, spaceID string) {
	t.Helper()

	gotSessionIndex, gotSpaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse drive route: %v", err)
	}
	if gotSessionIndex != sessionIndex || gotSpaceID != spaceID {
		t.Fatalf(
			"expected drive route to remain session %d space %q, got session %d space %q at %s",
			sessionIndex,
			spaceID,
			gotSessionIndex,
			gotSpaceID,
			page.URL(),
		)
	}
}

func openDriveInviteDialog(t testing.TB, page playwright.Page) {
	t.Helper()

	waitForDriveSettled(t, page)
	if err := page.Locator(driveWelcomeSelector).WaitFor(); err != nil {
		t.Fatalf("wait for drive welcome guidance: %v", err)
	}
	if err := page.Locator(driveInviteCTASelector + ":not([disabled])").First().Click(); err != nil {
		t.Fatalf("click drive invite CTA: %v", err)
	}
	dialog := page.Locator("[role='dialog']:has-text('Add User')").First()
	if err := dialog.WaitFor(); err != nil {
		t.Fatalf("wait for Add User dialog: %v", err)
	}
	if err := page.Keyboard().Press("Escape"); err != nil {
		t.Fatalf("close Add User dialog: %v", err)
	}
	if err := dialog.WaitFor(playwright.LocatorWaitForOptions{
		State: playwright.WaitForSelectorStateHidden,
	}); err != nil {
		t.Fatalf("wait for Add User dialog to close: %v", err)
	}
}
