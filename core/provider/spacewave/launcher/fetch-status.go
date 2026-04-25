package spacewave_launcher

import (
	"time"

	"github.com/aperturerobotics/controllerbus/directive"
)

// FetchStatus is a read-only snapshot of the launcher controller's DistConfig
// fetch state. Instances are immutable: transitions swap in a new pointer so
// equality by pointer identity doubles as a change signal.
type FetchStatus struct {
	// Fetching is true while an endpoint fetch is in flight.
	Fetching bool
	// HasConfig is true once the controller has a non-empty DistConfig
	// (either loaded from disk, the built-in default, or a successful fetch).
	HasConfig bool
	// LastErr is the most recent endpoint-fetch error string, or empty when
	// the last attempt succeeded or no fetch has run yet.
	LastErr string
	// Attempts is the number of endpoint rounds tried since the controller
	// started. It resets to 0 on successful fetch.
	Attempts uint32
	// NextRetryAt is the wall-clock time of the next scheduled retry. Zero
	// when no retry is pending (either fetch in flight or idle).
	NextRetryAt time.Time
}

// WatchLauncherFetchStatus is a directive that emits FetchStatus snapshots
// for a launcher controller matching ProjectID. The resolver pushes a new
// value on every state transition and removes the prior value.
type WatchLauncherFetchStatus interface {
	// Directive indicates WatchLauncherFetchStatus is a directive.
	directive.Directive

	// WatchLauncherFetchStatusProjectID returns the project id to filter on.
	// Empty to match any launcher controller on the bus.
	WatchLauncherFetchStatusProjectID() string
}

// WatchLauncherFetchStatusValue is the result type for
// WatchLauncherFetchStatus.
type WatchLauncherFetchStatusValue = *FetchStatus

// watchLauncherFetchStatus implements WatchLauncherFetchStatus.
type watchLauncherFetchStatus struct {
	projectID string
}

// NewWatchLauncherFetchStatus constructs the directive.
func NewWatchLauncherFetchStatus(projectID string) WatchLauncherFetchStatus {
	return &watchLauncherFetchStatus{projectID: projectID}
}

// Validate validates the directive.
func (d *watchLauncherFetchStatus) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *watchLauncherFetchStatus) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// WatchLauncherFetchStatusProjectID returns the project id to match.
func (d *watchLauncherFetchStatus) WatchLauncherFetchStatusProjectID() string {
	return d.projectID
}

// IsEquivalent checks if the other directive is equivalent.
func (d *watchLauncherFetchStatus) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(WatchLauncherFetchStatus)
	if !ok {
		return false
	}
	return d.WatchLauncherFetchStatusProjectID() ==
		od.WatchLauncherFetchStatusProjectID()
}

// Superceeds checks if the directive overrides another.
func (d *watchLauncherFetchStatus) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *watchLauncherFetchStatus) GetName() string {
	return "WatchLauncherFetchStatus"
}

// GetDebugVals returns the directive arguments stringified.
func (d *watchLauncherFetchStatus) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.projectID != "" {
		vals["project-id"] = []string{d.projectID}
	}
	return vals
}

// _ is a type assertion
var _ WatchLauncherFetchStatus = ((*watchLauncherFetchStatus)(nil))
