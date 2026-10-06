//go:build !skip_e2e && !js

package wasm

import (
	"strings"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
)

// TestPairingStorageQuotaJourney uses Chromium's real origin quota to reject
// enrollment writes after approval, without publishing a successful attachment.
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

	// Confirm pairing and require a quota failure.
	confirmPairingPages(t, a.Page(), b.Page())
	if err := b.Page().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Pairing failed", Exact: new(true)}).WaitFor(); err != nil {
		body, _ := b.Page().Locator("body").InnerText()
		t.Fatalf("storage failure: %v; page: %s", err, body)
	}
	body, err := b.Page().Locator("body").InnerText()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.ToLower(body), "quota") && !strings.Contains(strings.ToLower(body), "storage") {
		t.Fatalf("pairing failure did not identify storage: %s", body)
	}
	if strings.Contains(body, "Account connected") {
		t.Fatal("storage failure claimed successful account attachment")
	}
}
