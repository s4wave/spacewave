package forge_job

import (
	"slices"

	forge_value "github.com/aperturerobotics/forge/value"
	"github.com/pkg/errors"
)

// ErrUnknownState is returned if the state was unknown/unhandled.
var ErrUnknownState = errors.New("unexpected or unhandled state")

// Validate checks the execution state is within known values.
func (s State) Validate(allowUnknown bool) error {
	if s == State_JobState_UNKNOWN {
		if allowUnknown {
			return nil
		}
	}
	switch s {
	case State_JobState_PENDING:
	case State_JobState_RUNNING:
	case State_JobState_COMPLETE:
	default:
		return errors.Wrap(ErrUnknownState, s.String())
	}

	return nil
}

// EnsureMatches checks if the state matches or returns an error.
func (s State) EnsureMatches(sts ...State) error {
	var match bool
	if slices.Contains(sts, s) {
		match = true
	}
	if !match {
		return errors.Wrapf(
			forge_value.ErrUnknownState,
			"%s", s.String(),
		)
	}
	return nil
}
