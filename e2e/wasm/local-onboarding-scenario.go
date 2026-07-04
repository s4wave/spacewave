//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	session_core "github.com/s4wave/spacewave/core/session"
)

// LocalOnboardingScenario records the local session created by the
// quickstart flow and completed through the local onboarding UI.
type LocalOnboardingScenario struct {
	session      *TestSession
	sessionIndex uint32
	spaceID      string
}

// CreateLocalOnboardingScenario creates a local drive, follows the setup
// banner through the plan and setup pages, downloads a backup PEM, and sets
// the session lock mode to PIN.
func CreateLocalOnboardingScenario(t testing.TB, h *Harness, session *TestSession) *LocalOnboardingScenario {
	t.Helper()

	page := session.Page()
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	NavigateHash(t, h, page, "#/quickstart/drive")
	CompleteDriveIntroWizard(t, page)
	WaitForDriveReady(t, h, page)
	AssertSetupBannerHidden(t, page)
	UploadViaPicker(t, page, []playwright.InputFile{
		{
			Name:     "setup-banner-threshold.bin",
			MimeType: "application/octet-stream",
			Buffer:   setupBannerThresholdBuffer(),
		},
	})
	WaitForDriveEntry(t, page, "setup-banner-threshold.bin")

	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse local onboarding route: %v", err)
	}

	WaitForSetupBanner(t, h, page)
	if err := page.Locator("text=Finish setting up your account").First().Click(); err != nil {
		failWithPageBody(t, page, "open setup banner", err)
	}

	WaitForPlanPage(t, h, page)
	if err := page.Locator("button:visible:has-text('Continue with local storage')").First().Click(); err != nil {
		failWithPageBody(t, page, "choose local storage plan", err)
	}

	WaitForLocalSetupPage(t, page)
	CompleteLocalSetupPemStep(t, page)
	CompleteLocalSetupLockStep(t, page)
	WaitForLocalSetupComplete(t, page)
	WaitForSessionLockMode(t, h, session, sessionIndex, session_core.SessionLockMode_SESSION_LOCK_MODE_PIN_ENCRYPTED)

	return &LocalOnboardingScenario{
		session:      session,
		sessionIndex: sessionIndex,
		spaceID:      spaceID,
	}
}

// CompleteDriveIntroWizard opens the raw files browser for both current Drive
// wrapper quickstarts and legacy wizard-indexed quickstarts.
func CompleteDriveIntroWizard(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		const deadline = Date.now() + 120000
		for (;;) {
			const browser = document.querySelector('[data-testid="unixfs-browser"]')
			if (browser) return null
			const text = document.body.textContent ?? ''
			if (text.includes('Your Drive is ready')) {
				const open = Array.from(document.querySelectorAll('button')).find((button) =>
					button.textContent?.includes('Open files')
				)
				if (open instanceof HTMLButtonElement) {
					open.click()
				}
			}
			if (Date.now() > deadline) {
				throw new Error('Drive file browser did not appear')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
	}`)
	if err != nil {
		failWithPageBody(t, page, "open drive files", err)
	}
}

// AssertSetupBannerHidden verifies the setup banner does not render before the
// session has enough local storage to justify the onboarding nudge.
func AssertSetupBannerHidden(t testing.TB, page playwright.Page) {
	t.Helper()

	visible, err := page.Locator("text=Finish setting up your account").First().IsVisible()
	if err != nil {
		failWithPageBody(t, page, "check setup banner hidden", err)
	}
	if visible {
		failWithPageBody(t, page, "setup banner hidden before storage threshold", nil)
	}
}

// WaitForDriveEntry waits for a named Drive row to render.
func WaitForDriveEntry(t testing.TB, page playwright.Page, name string) {
	t.Helper()

	err := page.Locator("[data-testid='unixfs-browser'] [role='row']:has-text('" + name + "')").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	)
	if err != nil {
		failWithPageBody(t, page, "wait for drive entry "+name, err)
	}
}

func setupBannerThresholdBuffer() []byte {
	buf := make([]byte, 11*1024*1024)
	var x uint32 = 1
	for i := range buf {
		x = x*1664525 + 1013904223
		buf[i] = byte(x >> 24)
	}
	return buf
}

// GetSession returns the owning test session.
func (s *LocalOnboardingScenario) GetSession() *TestSession { return s.session }

// GetSessionIndex returns the 1-based session index from the quickstart route.
func (s *LocalOnboardingScenario) GetSessionIndex() uint32 { return s.sessionIndex }

// GetSpaceID returns the created space identifier from the quickstart route.
func (s *LocalOnboardingScenario) GetSpaceID() string { return s.spaceID }

// WaitForSetupBanner waits for the local setup banner to render.
func WaitForSetupBanner(t testing.TB, h *Harness, page playwright.Page) {
	t.Helper()

	if _, err := page.Evaluate(h.Script("wait-for-setup-banner.ts"), map[string]any{
		"deadlineMs": 120000,
	}); err != nil {
		failWithPageBody(t, page, "wait for setup banner", err)
	}
}

// WaitForPlanPage waits for the plan selection page to render.
func WaitForPlanPage(t testing.TB, h *Harness, page playwright.Page) {
	t.Helper()

	if _, err := page.Evaluate(h.Script("wait-for-plan-page.ts"), map[string]any{
		"deadlineMs": 120000,
	}); err != nil {
		failWithPageBody(t, page, "wait for plan page", err)
	}
}

// WaitForLocalSetupPage waits for the local setup wizard to render.
func WaitForLocalSetupPage(t testing.TB, page playwright.Page) {
	t.Helper()

	err := page.Locator("text=Your data lives on this device").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	)
	if err != nil {
		failWithPageBody(t, page, "wait for local setup page", err)
	}
}

// CompleteLocalSetupPemStep downloads the local backup PEM and waits for the
// setup state to mark the backup step complete.
func CompleteLocalSetupPemStep(t testing.TB, page playwright.Page) {
	t.Helper()

	if err := page.Locator("button:visible:has-text('Download a backup key')").First().Click(); err != nil {
		failWithPageBody(t, page, "expand backup key step", err)
	}
	if err := page.Locator("input:visible[placeholder='Choose a password for recovery']").Fill("local-onboarding-recovery-password"); err != nil {
		failWithPageBody(t, page, "fill recovery password", err)
	}
	if err := page.Locator("button:visible:has-text('Download backup .pem')").First().Click(); err != nil {
		failWithPageBody(t, page, "download backup pem", err)
	}
	err := page.Locator("text=Backup key saved").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	)
	if err != nil {
		failWithPageBody(t, page, "wait for backup key saved", err)
	}
}

// CompleteLocalSetupLockStep sets the local session lock mode to PIN and waits
// for the setup state to mark the lock step complete.
func CompleteLocalSetupLockStep(t testing.TB, page playwright.Page) {
	t.Helper()

	if err := page.Locator("button:visible:has-text('Set a PIN lock')").First().Click(); err != nil {
		failWithPageBody(t, page, "expand pin lock step", err)
	}
	if err := page.Locator("button:visible:has-text('Enter PIN on each app launch')").First().Click(); err != nil {
		failWithPageBody(t, page, "select pin lock mode", err)
	}
	if err := page.Locator("input:visible[placeholder='Enter PIN']").Fill("2468"); err != nil {
		failWithPageBody(t, page, "fill pin", err)
	}
	if err := page.Locator("input:visible[placeholder='Confirm PIN']").Fill("2468"); err != nil {
		failWithPageBody(t, page, "fill confirm pin", err)
	}
	if err := page.Locator("button:visible:has-text('Set lock mode')").First().Click(); err != nil {
		failWithPageBody(t, page, "set lock mode", err)
	}
	err := page.Locator("text=PIN lock enabled").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	)
	if err != nil {
		failWithPageBody(t, page, "wait for pin lock enabled", err)
	}
}

// WaitForLocalSetupComplete waits until both local onboarding cards show their
// completed state and the setup banner is no longer visible.
func WaitForLocalSetupComplete(t testing.TB, page playwright.Page) {
	t.Helper()

	_, err := page.Evaluate(`async () => {
		const deadline = Date.now() + 120000
		for (;;) {
			const text = document.body.textContent ?? ''
			const complete =
				text.includes('Backup key saved') &&
				text.includes('PIN lock enabled') &&
				!text.includes('Finish setting up your account')
			if (complete) return null
			if (Date.now() > deadline) {
				throw new Error('local setup did not reach complete state')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
	}`)
	if err != nil {
		failWithPageBody(t, page, "wait for local setup complete", err)
	}
}

// WaitForSessionLockMode waits for the mounted session to report the expected
// lock mode through the SDK lock-state stream.
func WaitForSessionLockMode(
	t testing.TB,
	h *Harness,
	session *TestSession,
	sessionIndex uint32,
	want session_core.SessionLockMode,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(h.Context(), 60*time.Second)
	defer cancel()

	sdk, err := session.MountSessionByIdx(ctx, sessionIndex)
	if err != nil {
		t.Fatalf("mount session %d for lock-state check: %v", sessionIndex, err)
	}
	defer sdk.Release()

	strm, err := sdk.WatchLockState(ctx)
	if err != nil {
		t.Fatalf("watch lock state: %v", err)
	}
	defer strm.Close()

	for {
		resp, err := strm.Recv()
		if err != nil {
			t.Fatalf("recv lock state: %v", err)
		}
		if resp.GetMode() == want {
			return
		}
	}
}

func failWithPageBody(t testing.TB, page playwright.Page, label string, err error) {
	t.Helper()

	body, bodyErr := page.Locator("body").TextContent()
	if bodyErr != nil {
		body = "failed to read body text: " + bodyErr.Error()
	}
	t.Fatalf("%s: %v\nurl: %s\nbody: %s", label, err, page.URL(), trimPageText(body))
}
