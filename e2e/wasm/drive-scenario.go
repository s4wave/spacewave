//go:build !skip_e2e && !js

package wasm

import "testing"

// DriveScenario records the owned drive created by the quickstart flow.
type DriveScenario struct {
	session      *TestSession
	sessionIndex uint32
	spaceID      string
}

// CreateDriveScenario creates a drive in a fresh harness session.
func CreateDriveScenario(t testing.TB, h *Harness, session *TestSession) *DriveScenario {
	t.Helper()

	// Capture the identity produced by this document's setup.
	sessionIndex, spaceID := session.createDriveScenario(t, h)
	return &DriveScenario{session: session, sessionIndex: sessionIndex, spaceID: spaceID}
}

// createDriveScenario drives the current document through the scenario setup.
func (s *TestSession) createDriveScenario(t testing.TB, h *Harness) (uint32, string) {
	// Mark the scenario helper as a test helper.
	t.Helper()

	// Drive the browser through the drive quickstart flow until the shell loads.
	page := s.page
	WaitForApp(t, page)
	EnableQuickstartTimingLogs(t, page)
	h.navigateHash(t, page, "#/quickstart/drive")
	WaitForDriveShell(t, page)

	// Parse the created drive's identity out of the quickstart route.
	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse drive route: %v", err)
	}

	return sessionIndex, spaceID
}

// GetSession returns the owning test s.
func (s *DriveScenario) GetSession() *TestSession { return s.session }

// GetSessionIndex returns the 1-based session index from the quickstart route.
func (s *DriveScenario) GetSessionIndex() uint32 { return s.sessionIndex }

// GetSpaceID returns the created space identifier from the quickstart route.
func (s *DriveScenario) GetSpaceID() string { return s.spaceID }
