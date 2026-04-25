//go:build !js

package wasm

import (
	"testing"
)

// DriveScenario records the owned drive created by the quickstart flow.
type DriveScenario struct {
	session      *TestSession
	sessionIndex uint32
	spaceID      string
}

// CreateDriveScenario creates a drive in a fresh harness session.
func CreateDriveScenario(t testing.TB, h *Harness, session *TestSession) *DriveScenario {
	t.Helper()

	page := session.Page()
	WaitForApp(t, page)
	NavigateHash(t, h, page, "#/quickstart/drive")
	WaitForDriveShell(t, page)

	sessionIndex, spaceID, err := parseQuickstartRoute(page.URL())
	if err != nil {
		t.Fatalf("parse drive route: %v", err)
	}

	return &DriveScenario{
		session:      session,
		sessionIndex: sessionIndex,
		spaceID:      spaceID,
	}
}

// GetSession returns the owning test session.
func (s *DriveScenario) GetSession() *TestSession { return s.session }

// GetSessionIndex returns the 1-based session index from the quickstart route.
func (s *DriveScenario) GetSessionIndex() uint32 { return s.sessionIndex }

// GetSpaceID returns the created space identifier from the quickstart route.
func (s *DriveScenario) GetSpaceID() string { return s.spaceID }
