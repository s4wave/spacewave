package forge_pass

import (
	"slices"

	"github.com/pkg/errors"
	forge_value "github.com/s4wave/spacewave/forge/value"
)

// ErrUnknownState is returned if the state was unknown/unhandled.
var ErrUnknownState = errors.New("unexpected or unhandled state")

// Validate checks the execution state is within known values.
func (s State) Validate(allowUnknown bool) error {
	if s == State_PassState_UNKNOWN && allowUnknown {
		return nil
	}
	switch s {
	case State_PassState_PENDING:
	case State_PassState_RUNNING:
	case State_PassState_CHECKING:
	case State_PassState_COMPLETE:
	case State_PassState_CANCELING:
	default:
		return errors.Wrap(ErrUnknownState, s.String())
	}

	return nil
}

// EnsureMatches checks if the state matches or returns an error.
func (s State) EnsureMatches(sts ...State) error {
	if slices.Contains(sts, s) {
		return nil
	}
	return errors.Wrapf(
		forge_value.ErrUnknownState,
		"%s", s.String(),
	)
}
