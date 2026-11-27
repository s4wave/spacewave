package forge_pass

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	forge_execution "github.com/s4wave/spacewave/forge/execution"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// newExecState creates a new ExecState from an Execution.
func newExecState(objKey string, e *forge_execution.Execution) *ExecState {
	if e == nil {
		return nil
	}

	return &ExecState{
		ObjectKey:      objKey,
		ExecutionState: e.ExecutionState,
		PeerId:         e.PeerId,
		Timestamp:      e.GetTimestamp().CloneVT(),
		ValueSet:       e.GetValueSet().Clone(),
		Result:         e.GetResult().Clone(),
	}
}

// IsNil checks if the object is nil.
func (s *ExecState) IsNil() bool {
	return s == nil
}

// Validate checks if the exec state looks valid.
func (s *ExecState) Validate() error {
	// Validate the execution state, peer ID, timestamp, value set, and result.
	if err := s.GetExecutionState().Validate(false); err != nil {
		return err
	}
	if _, err := s.parsePeerID(); err != nil {
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

// parsePeerID parses the peer ID field.
func (s *ExecState) parsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(s.GetPeerId())
}

// _ is a type assertion
var _ block.NamedSubBlock = (*ExecState)(nil)
