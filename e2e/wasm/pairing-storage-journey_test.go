//go:build !skip_e2e && !js

package wasm

import (
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// TestPairingStorageQuotaJourney uses Chromium's real origin quota to reject
// the paired account's Space copy and requires the account's sync status to
// report the full storage. The attachment itself may fit in storage Chromium
// already granted the origin.
func TestPairingStorageQuotaJourney(t *testing.T) {
	// Open two Chromium sessions and start pairing from Drive.
	h := harness(t)
	if h.browserName != "chromium" {
		t.Skip("origin quota override requires Chromium")
	}
	a, b := h.NewCleanSession(t), h.NewCleanSession(t)
	drive := CreateDriveScenario(t, h, a)
	WaitForDriveReady(t, h, a.Page())
	startPairingPages(t, a.Page(), b.Page(), drive.GetSessionIndex(), "#/")
	if err := b.Page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Yes, they match", Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}

	// Override the second page's storage quota.
	cdp, err := b.BrowserContext().NewCDPSession(b.Page())
	if err != nil {
		t.Fatal(err)
	}
	defer cdp.Detach()
	origin, err := b.Page().Evaluate("() => location.origin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := cdp.Send("Storage.overrideQuotaForOrigin", map[string]any{"origin": origin, "quotaSize": 1}); err != nil {
		t.Fatal(err)
	}
	defer cdp.Send("Storage.overrideQuotaForOrigin", map[string]any{"origin": origin})

	// Confirm pairing, open the account, and require the copy to report
	// full storage.
	confirmPairingPages(t, a.Page(), b.Page())
	open := b.Page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Open account", Exact: new(true)})
	if err := open.Click(); err != nil {
		body, _ := b.Page().Locator("body").InnerText()
		t.Fatalf("open paired account: %v; page: %s", err, body)
	}
	status := b.Page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Session sync status: Storage full", Exact: new(true)})
	if err := status.WaitFor(); err != nil {
		body, _ := b.Page().Locator("body").InnerText()
		t.Fatalf("storage failure: %v; page: %s", err, body)
	}
}
