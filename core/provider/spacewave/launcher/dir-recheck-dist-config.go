package spacewave_launcher

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
)

// RecheckDistConfig is a directive to trigger an immediate re-fetch of the
// dist config by the launcher controller. The resolver emits a single true
// value once the recheck has been dispatched.
type RecheckDistConfig interface {
	// Directive indicates RecheckDistConfig is a directive.
	directive.Directive

	// RecheckDistConfigProjectID returns the project id to filter on.
	// Empty to match any running launcher controller.
	RecheckDistConfigProjectID() string
}

// RecheckDistConfigValue is the result type for RecheckDistConfig.
type RecheckDistConfigValue = bool

// ExRecheckDistConfig dispatches a RecheckDistConfig directive and waits for
// at least one launcher controller to acknowledge it.
func ExRecheckDistConfig(ctx context.Context, b bus.Bus, projectID string) error {
	dir := NewRecheckDistConfig(projectID)
	_, _, dirRef, err := bus.ExecWaitValue[RecheckDistConfigValue](ctx, b, dir, nil, nil, nil)
	if dirRef != nil {
		dirRef.Release()
	}
	return err
}

// recheckDistConfig implements RecheckDistConfig.
type recheckDistConfig struct {
	projectID string
}

// NewRecheckDistConfig constructs a new RecheckDistConfig directive.
func NewRecheckDistConfig(projectID string) RecheckDistConfig {
	return &recheckDistConfig{projectID: projectID}
}

// Validate validates the directive.
func (d *recheckDistConfig) Validate() error {
	return nil
}

// GetValueOptions returns options relating to value handling.
func (d *recheckDistConfig) GetValueOptions() directive.ValueOptions {
	return directive.ValueOptions{
		UnrefDisposeDur:            time.Millisecond * 100,
		UnrefDisposeEmptyImmediate: true,
	}
}

// RecheckDistConfigProjectID returns the project id to match.
func (d *recheckDistConfig) RecheckDistConfigProjectID() string {
	return d.projectID
}

// IsEquivalent checks if the other directive is equivalent.
func (d *recheckDistConfig) IsEquivalent(other directive.Directive) bool {
	od, ok := other.(RecheckDistConfig)
	if !ok {
		return false
	}
	return d.RecheckDistConfigProjectID() == od.RecheckDistConfigProjectID()
}

// Superceeds checks if the directive overrides another.
func (d *recheckDistConfig) Superceeds(other directive.Directive) bool {
	return false
}

// GetName returns the directive's type name.
func (d *recheckDistConfig) GetName() string {
	return "RecheckDistConfig"
}

// GetDebugVals returns the directive arguments stringified.
func (d *recheckDistConfig) GetDebugVals() directive.DebugValues {
	vals := directive.DebugValues{}
	if d.projectID != "" {
		vals["project-id"] = []string{d.projectID}
	}
	return vals
}

// _ is a type assertion
var _ RecheckDistConfig = ((*recheckDistConfig)(nil))
