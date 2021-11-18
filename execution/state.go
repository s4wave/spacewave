package forge_execution

import "github.com/pkg/errors"

// Validate checks the execution state is within known values.
func (s State) Validate(allowUnknown bool) error {
	if s == State_ExecutionState_UNKNOWN {
		if allowUnknown {
			return nil
		}
	}
	switch s {
	case State_ExecutionState_PENDING:
	case State_ExecutionState_RUNNING:
	case State_ExecutionState_COMPLETE:
	default:
		return errors.Wrap(ErrUnknownState, s.String())
	}

	return nil
}
