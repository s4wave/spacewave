package forge_pass

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	forge_execution "github.com/aperturerobotics/forge/execution"
	"github.com/aperturerobotics/hydra/block"
	"github.com/pkg/errors"
)

// NewExecState creates a new ExecState from an Execution.
func NewExecState(objKey string, e *forge_execution.Execution) *ExecState {
	if e == nil {
		return nil
	}

	return &ExecState{
		ObjectKey:      objKey,
		ExecutionState: e.ExecutionState,
		PeerId:         e.PeerId,
		Timestamp:      e.GetTimestamp().Clone(),
		ValueSet:       e.GetValueSet().Clone(),
		Result:         e.GetResult().Clone(),
	}
}

// IsNil checks if the object is nil.
func (e *ExecState) IsNil() bool {
	return e == nil
}

// Validate checks if the exec state looks valid.
func (s *ExecState) Validate() error {
	if err := s.GetExecutionState().Validate(false); err != nil {
		return err
	}
	if _, err := s.ParsePeerID(); err != nil {
		return err
	}
	if err := s.GetTimestamp().Validate(false); err != nil {
		return err
	}
	if err := s.GetValueSet().Validate(); err != nil {
		return errors.Wrap(err, "value_set")
	}
	if err := s.GetResult().Validate(); err != nil {
		return errors.Wrap(err, "result")
	}
	return nil
}

// GetName returns the name of the ref.
func (s *ExecState) GetName() string {
	return s.GetObjectKey()
}

// ParsePeerID parses the peer ID field.
func (s *ExecState) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(s.GetPeerId())
}

// MatchesExecution checks if the details in the ExecState match the Execution.
func (s *ExecState) MatchesExecution(exec *forge_execution.Execution) bool {
	switch {
	case s.GetExecutionState() != exec.GetExecutionState():
	case s.GetPeerId() != exec.GetPeerId():
	case !s.GetTimestamp().Equals(exec.GetTimestamp()):
	default:
		return true
	}
	return false
}

// Equals checks if the exec state is the same as the other exec state.
func (s *ExecState) Equals(ot *ExecState) bool {
	switch {
	case s.GetObjectKey() != ot.GetObjectKey():
	case s.GetExecutionState() != ot.GetExecutionState():
	case s.GetPeerId() != ot.GetPeerId():
	case !s.GetTimestamp().Equals(ot.GetTimestamp()):
	default:
		return true
	}
	return false
}

// _ is a type assertion
var _ block.NamedSubBlock = ((*ExecState)(nil))
