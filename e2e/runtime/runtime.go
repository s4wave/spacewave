// Package runtime defines the user-facing contract shared by every E2E
// runtime adapter.
package runtime

import (
	"errors"
	"fmt"
)

// SessionRequirement controls the state boundary before a scenario runs.
type SessionRequirement string

const (
	SessionAny          SessionRequirement = ""
	SessionFresh        SessionRequirement = "fresh-session"
	SessionFreshInstall SessionRequirement = "fresh-install"
)

// Valid reports whether the requirement is one of the supported boundaries.
func (r SessionRequirement) Valid() bool {
	return r == SessionAny || r == SessionFresh || r == SessionFreshInstall
}

// Event names a readiness transition owned by the runtime under test.
type Event string

const (
	EventAppReady           Event = "app-ready"
	EventDriveReady         Event = "drive-ready"
	EventDriveSettled       Event = "drive-settled"
	EventContentReady       Event = "content-ready"
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
	ID() string
}

type Runtime interface {
	Name() string
	ResetSession(requirement SessionRequirement) error

	OpenRoute(route string) error
	ClickControl(control string) error
	DoubleClickContent(content string) error
	Type(field string, value string) error
	UploadFile(target string, file File) error
	MoveContent(source string, target string) error
	DeleteSpace() error

	ExpectVisible(content string) error
	ExpectAbsent(content string) error
	ExpectRoute(route string) error
	WaitForEvent(event Event) error

	OpenSecondTab(route string) (Tab, error)
	BackgroundTab(tab Tab) error
	ReloadPage() error
	RestartWorkerHost() error
}

// SkipError records an adapter capability or environment limitation that
// should produce a SKIP row rather than a failed scenario.
type SkipError struct {
	Control string
	Reason  string
}

func (e *SkipError) Error() string {
	if e.Control == "" {
		return e.Reason
	}
	return fmt.Sprintf("%s: %s", e.Control, e.Reason)
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
