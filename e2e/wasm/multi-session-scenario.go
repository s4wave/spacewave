//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
)

const multiSessionPIN = "2468"

// MultiSessionScenario records two local sessions created inside one browser
// context for session switching and lock/unlock trajectories.
type MultiSessionScenario struct {
	h                  *Harness
	session            *TestSession
	firstSessionIndex  uint32
	firstSpaceID       string
	secondSessionIndex uint32
}

// CreateMultiSessionScenario creates a PIN-backed local drive session and a
// second local session in the same browser context.
func CreateMultiSessionScenario(t testing.TB, h *Harness, session *TestSession) *MultiSessionScenario {
	// Report failures at the caller.
	t.Helper()

	// Finish local onboarding in the first session.
	first := CreateLocalOnboardingScenario(t, h, session)
	page := session.Page()

	// Open a second local quickstart and require a new session index.
	NavigateHash(t, h, page, "#/quickstart/local")
	secondSessionIndex := waitForSessionRoute(t, page)
	if secondSessionIndex == first.GetSessionIndex() {
		t.Fatalf("expected quickstart/local to create a second session, got %d twice", secondSessionIndex)
	}

	// Record both sessions and wait for the local badge.
	scenario := &MultiSessionScenario{
		h:                  h,
		session:            session,
		firstSessionIndex:  first.GetSessionIndex(),
		firstSpaceID:       first.GetSpaceID(),
		secondSessionIndex: secondSessionIndex,
	}
	scenario.waitForSessionCount(t, 2)
	scenario.WaitForLocalBadge(t)
	return scenario
}

// GetSession returns the browser context and Resource SDK session.
func (s *MultiSessionScenario) GetSession() *TestSession { return s.session }

// GetFirstSessionIndex returns the PIN-backed drive session index.
func (s *MultiSessionScenario) GetFirstSessionIndex() uint32 { return s.firstSessionIndex }

// GetSecondSessionIndex returns the second local session index.
func (s *MultiSessionScenario) GetSecondSessionIndex() uint32 { return s.secondSessionIndex }

// GetFirstSpaceID returns the drive Space created for the first session.
func (s *MultiSessionScenario) GetFirstSpaceID() string { return s.firstSpaceID }

// ExitToSessionSelector follows the same selector route used by local session
// switch and lock commands.
func (s *MultiSessionScenario) ExitToSessionSelector(t testing.TB) {
	// Report failures at the caller.
	t.Helper()

	// Open the session selector.
	page := s.session.Page()
	NavigateHash(t, s.h, page, "#/sessions")
	err := page.Locator("[data-testid='session-selector']").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	)
	if err != nil {
		failWithPageBody(t, page, "wait for session selector", err)
	}
}

// SwitchToSession opens a session from the selector and waits for its route.
func (s *MultiSessionScenario) SwitchToSession(t testing.TB, sessionIndex uint32) {
	// Report failures at the caller.
	t.Helper()

	// Click the session card and require its route.
	s.ExitToSessionSelector(t)
	page := s.session.Page()
	card := page.Locator(
		"[data-testid='session-card'][data-session-index='" + strconv.FormatUint(uint64(sessionIndex), 10) + "']",
	).First()
	if err := card.Click(); err != nil {
		failWithPageBody(t, page, "click session card", err)
	}
	got := waitForSessionRoute(t, page)
	if got != sessionIndex {
		t.Fatalf("session switch opened index %d, want %d", got, sessionIndex)
	}
}

// WaitForLocalBadge waits for the account bottom-bar badge to report LOCAL.
func (s *MultiSessionScenario) WaitForLocalBadge(t testing.TB) {
	// Report failures at the caller.
	t.Helper()

	// Unlock with the PIN when the badge is behind the lock.
	page := s.session.Page()
	state, err := waitForLocalBadgeOrPIN(page)
	if err != nil {
		failWithPageBody(t, page, "wait for session provider badge or PIN unlock", err)
	}
	if state == "pin" {
		s.submitVisiblePIN(t)
	}
	s.waitForLocalBadge(t)
}

func (s *MultiSessionScenario) waitForLocalBadge(t testing.TB) {
	// Report failures at the caller.
	t.Helper()

	// Require the visible provider badge to say LOCAL.
	page := s.session.Page()
	badge := page.Locator("[data-testid='session-account-provider-badge']").First()
	if err := badge.WaitFor(playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)}); err != nil {
		failWithPageBody(t, page, "wait for session provider badge", err)
	}
	text, err := badge.TextContent()
	if err != nil {
		failWithPageBody(t, page, "read session provider badge", err)
	}
	if strings.TrimSpace(text) != "LOCAL" {
		t.Fatalf("expected LOCAL session badge, got %q", strings.TrimSpace(text))
	}
}

// OpenFirstDrive opens the first session's drive Space as a nested route.
func (s *MultiSessionScenario) OpenFirstDrive(t testing.TB) {
	t.Helper()

	page := s.session.Page()
	NavigateHash(
		t,
		s.h,
		page,
		"#/u/"+strconv.FormatUint(uint64(s.firstSessionIndex), 10)+"/so/"+s.firstSpaceID,
	)
	WaitForDriveShell(t, page)
}

// LockFirstSessionAtNestedRoute locks the first session while the browser is on
// its nested drive route and waits for the PIN overlay.
func (s *MultiSessionScenario) LockFirstSessionAtNestedRoute(t testing.TB) {
	// Report failures at the caller.
	t.Helper()

	// Open the first session's Drive.
	s.OpenFirstDrive(t)

	// Bound the lock request.
	ctx, cancel := context.WithTimeout(s.h.Context(), 30*time.Second)
	defer cancel()

	// Mount the first session.
	sdk, err := s.session.MountSessionByIdx(ctx, s.firstSessionIndex)
	if err != nil {
		t.Fatalf("mount first session for lock: %v", err)
	}
	defer sdk.Release()

	// Lock the session and wait for the PIN overlay.
	if err := sdk.LockSession(ctx); err != nil {
		t.Fatalf("lock first session: %v", err)
	}
	WaitForPinUnlockOverlay(t, s.session.Page())
}

// UnlockVisiblePIN unlocks the currently visible PIN overlay.
func (s *MultiSessionScenario) UnlockVisiblePIN(t testing.TB) {
	t.Helper()

	s.submitVisiblePIN(t)
	WaitForDriveShell(t, s.session.Page())
	if !strings.Contains(s.session.Page().URL(), "/u/"+strconv.FormatUint(uint64(s.firstSessionIndex), 10)+"/so/"+s.firstSpaceID) {
		t.Fatalf("unlock did not restore first nested route, got %q", s.session.Page().URL())
	}
}

func (s *MultiSessionScenario) submitVisiblePIN(t testing.TB) {
	// Report failures at the caller.
	t.Helper()

	// Fill and submit the visible PIN.
	page := s.session.Page()
	WaitForPinUnlockOverlay(t, page)
	if err := page.Locator("[data-testid='pin-unlock-input']").Fill(multiSessionPIN); err != nil {
		failWithPageBody(t, page, "fill pin unlock input", err)
	}
	if err := page.Locator("[data-testid='pin-unlock-submit']").Click(); err != nil {
		failWithPageBody(t, page, "submit pin unlock", err)
	}
}

func waitForLocalBadgeOrPIN(page playwright.Page) (string, error) {
	// Evaluate the page until the local badge or PIN overlay appears.
	raw, err := page.Evaluate(`async ({ deadlineMs }) => {
		const deadline = Date.now() + deadlineMs
		for (;;) {
			if (document.querySelector('[data-testid="session-account-provider-badge"]')) {
				return 'badge'
			}
			if (document.querySelector('[data-testid="pin-unlock-overlay"]')) {
				return 'pin'
			}
			if (Date.now() > deadline) {
				throw new Error('session provider badge or PIN unlock did not appear before deadline')
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
	}`, map[string]any{
		"deadlineMs": 120000,
	})
	if err != nil {
		return "", err
	}
	state, ok := raw.(string)
	if !ok {
		return "", errors.Errorf("session badge/PIN wait returned %T: %#v", raw, raw)
	}
	return state, nil
}

// WaitForPinUnlockOverlay waits for the session PIN unlock gate.
func WaitForPinUnlockOverlay(t testing.TB, page playwright.Page) {
	t.Helper()

	err := page.Locator("[data-testid='pin-unlock-overlay']").First().WaitFor(
		playwright.LocatorWaitForOptions{Timeout: playwright.Float(120000)},
	)
	if err != nil {
		failWithPageBody(t, page, "wait for pin unlock overlay", err)
	}
}

func waitForCount(ctx context.Context, next func(context.Context) (int, error), want int) error {
	for {
		count, err := next(ctx)
		if err != nil {
			return err
		}
		if count >= want {
			return nil
		}
	}
}

func (s *MultiSessionScenario) waitForSessionCount(t testing.TB, want int) {
	// Report failures at the caller.
	t.Helper()

	// Watch sessions for the expected count.
	ctx, cancel := context.WithTimeout(s.h.Context(), 30*time.Second)
	defer cancel()
	watch, err := s.session.Root().WatchSessions(ctx)
	if err != nil {
		t.Fatalf("watch sessions: %v", err)
	}
	defer func() {
		if err := watch.Close(); err != nil {
			t.Logf("close session watch: %v", err)
		}
	}()

	// Wait until the watched session count matches.
	if err := waitForCount(ctx, func(ctx context.Context) (int, error) {
		resp, err := watch.Recv()
		if err != nil {
			return 0, err
		}
		return len(resp.GetSessions()), nil
	}, want); err != nil {
		t.Fatalf("wait for %d sessions: %v", want, err)
	}
}

func waitForSessionRoute(t testing.TB, page playwright.Page) uint32 {
	// Report failures at the caller.
	t.Helper()

	// Wait until the page URL contains a session route.
	raw, err := page.Evaluate(`async ({ deadlineMs }) => {
		const deadline = Date.now() + deadlineMs
		for (;;) {
			if (/^#\/u\/\d+(?:\/|$)/.test(window.location.hash)) {
				return window.location.href
			}
			if (Date.now() > deadline) {
				throw new Error('session route did not appear before deadline: ' + window.location.hash)
			}
			await new Promise((resolve) => requestAnimationFrame(resolve))
		}
	}`, map[string]any{
		"deadlineMs": 120000,
	})
	if err != nil {
		failWithPageBody(t, page, "wait for session route", err)
	}

	// Parse the session index from that URL.
	rawURL, ok := raw.(string)
	if !ok {
		t.Fatalf("session route wait returned %T: %#v", raw, raw)
	}
	sessionIndex, err := parseSessionRoute(rawURL)
	if err != nil {
		t.Fatalf("parse session route: %v", err)
	}
	return sessionIndex
}

func parseSessionRoute(rawURL string) (uint32, error) {
	// Require a hash route.
	hashIdx := strings.Index(rawURL, "#")
	if hashIdx == -1 || hashIdx == len(rawURL)-1 {
		return 0, errors.New("missing hash route")
	}

	// Parse the session index from the user path.
	parts := strings.Split(strings.TrimPrefix(rawURL[hashIdx:], "#"), "/")
	if len(parts) < 3 || parts[1] != "u" {
		return 0, errors.Errorf("unexpected route %q", rawURL[hashIdx:])
	}
	idx, err := strconv.ParseUint(parts[2], 10, 32)
	if err != nil {
		return 0, errors.Wrap(err, "parse session index")
	}
	return uint32(idx), nil
}
