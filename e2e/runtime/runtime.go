// Package runtime defines the user-facing contract shared by every E2E
// runtime adapter.
package runtime

import "errors"

// SessionRequirement controls the state boundary before a scenario runs.
type SessionRequirement string

const (
	// SessionAny reuses the current warm session state.
	SessionAny SessionRequirement = ""
	// SessionFresh creates a fresh page in the current browser install.
	SessionFresh SessionRequirement = "fresh-session"
	// SessionFreshInstall creates a fresh browser install and page.
	SessionFreshInstall SessionRequirement = "fresh-install"
)

// Valid reports whether the requirement is one of the supported boundaries.
func (r SessionRequirement) Valid() bool {
	return r == SessionAny || r == SessionFresh || r == SessionFreshInstall
}

// Event names a readiness transition owned by the runtime under test.
type Event string

const (
	// EventAppReady waits for the application document to render.
	EventAppReady Event = "app-ready"
	// EventDriveReady waits for the Drive shell to finish its intro path.
	EventDriveReady Event = "drive-ready"
	// EventDriveSettled waits for Drive loading diagnostics to become idle.
	EventDriveSettled Event = "drive-settled"
	// EventContentReady waits for an opened content view to render.
	EventContentReady Event = "content-ready"
	// EventSpaceListConverged waits for a session-level Space list readback.
	EventSpaceListConverged Event = "space-list-converged"
)

// File is an in-memory upload fixture supplied through the user-action API.
type File struct {
	Name     string
	MIMEType string
	Contents []byte
}

// Tab is an opaque browser tab owned by a runtime adapter.
type Tab interface {
	// ID returns the runtime-specific tab identifier.
	ID() string
}

// Runtime is the scenario-facing API implemented by browser adapters.
type Runtime interface {
	// Name returns the runtime identifier used in reports.
	Name() string
	// ResetSession applies a declared state boundary before a scenario runs.
	ResetSession(requirement SessionRequirement) error

	// OpenRoute opens an application route without rebooting warm runtimes.
	OpenRoute(route string) error
	// ClickControl activates a named or selector-backed control.
	ClickControl(control string) error
	// DoubleClickContent opens visible content by name.
	DoubleClickContent(content string) error
	// Type enters a value into a named or selector-backed field.
	Type(field string, value string) error
	// UploadFile supplies an in-memory file to the runtime upload control.
	UploadFile(target string, file File) error
	// MoveContent moves one visible content row into another.
	MoveContent(source string, target string) error
	// DeleteSpace deletes the currently open Space and waits for completion.
	DeleteSpace() error

	// ExpectVisible waits until visible content appears.
	ExpectVisible(content string) error
	// ExpectAbsent waits until visible content disappears.
	ExpectAbsent(content string) error
	// ExpectRoute verifies the current route contains the expected path.
	ExpectRoute(route string) error
	// WaitForEvent waits for a named runtime readiness transition.
	WaitForEvent(event Event) error

	// OpenSecondTab opens another browser tab at a route in the same install.
	OpenSecondTab(route string) (Tab, error)
	// BackgroundTab moves a tab out of foreground focus when supported.
	BackgroundTab(tab Tab) error
	// ReloadPage reloads the active page without replacing the browser install.
	ReloadPage() error
	// RestartWorkerHost restarts the runtime worker host when supported.
	RestartWorkerHost() error
}

// SkipError records an adapter capability or environment limitation that
// should produce a SKIP row rather than a failed scenario.
type SkipError struct {
	Control string
	Reason  string
}

// Error returns the skip reason, prefixed by the unavailable control.
func (e *SkipError) Error() string {
	if e.Control == "" {
		return e.Reason
	}
	return e.Control + ": " + e.Reason
}

// Unsupported returns a skip error for an unavailable runtime control.
func Unsupported(control string, reason string) error {
	return &SkipError{Control: control, Reason: reason}
}

// IsUnsupported reports whether err requests a SKIP result.
func IsUnsupported(err error) bool {
	var skip *SkipError
	return errors.As(err, &skip)
}

// SkipReason returns the human-readable reason from a skip error.
func SkipReason(err error) string {
	var skip *SkipError
	if !errors.As(err, &skip) {
		return ""
	}
	return skip.Reason
}
