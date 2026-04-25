//go:build !js

package wasm

import (
	"testing"

	playwright "github.com/playwright-community/playwright-go"
)

// ForgeScenario records the forge environment created by the quickstart flow.
type ForgeScenario struct {
	session      *TestSession
	sessionIndex uint32
	spaceID      string
}

// CreateForgeScenario creates a forge environment in a fresh harness session.
func CreateForgeScenario(t testing.TB, h *Harness, session *TestSession) *ForgeScenario {
	t.Helper()

	page := session.Page()
	WaitForApp(t, page)
	NavigateHash(t, h, page, "#/quickstart/forge")
	WaitForForgeViewer(t, page)

	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse forge route: %v", err)
	}

	return &ForgeScenario{
		session:      session,
		sessionIndex: sessionIndex,
		spaceID:      spaceID,
	}
}

// GetSession returns the owning test session.
func (s *ForgeScenario) GetSession() *TestSession { return s.session }

// GetSessionIndex returns the 1-based session index from the quickstart route.
func (s *ForgeScenario) GetSessionIndex() uint32 { return s.sessionIndex }

// GetSpaceID returns the created space identifier from the quickstart route.
func (s *ForgeScenario) GetSpaceID() string { return s.spaceID }

// WaitForForgeViewer waits for a forge viewer shell to render.
func WaitForForgeViewer(t testing.TB, page playwright.Page) {
	t.Helper()

	err := page.Locator("[data-testid='forge-viewer']").First().WaitFor()
	if err != nil {
		t.Fatalf("wait for forge viewer: %v", err)
	}
}

// WaitForForgeReady waits for the forge dashboard to render with entity counts.
func WaitForForgeReady(t testing.TB, h *Harness, page playwright.Page) {
	t.Helper()

	WaitForForgeViewer(t, page)

	_, err := page.Evaluate(h.Script("wait-for-forge.ts"), map[string]any{
		"deadlineMs": 120000,
	})
	if err != nil {
		t.Fatalf("wait for forge ready: %v", err)
	}
}
