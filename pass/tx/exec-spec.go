package pass_tx

import (
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/util/confparse"
	"github.com/pkg/errors"
)

// ValidateExecSpecs validates an exec spec set.
func ValidateExecSpecs(execSpecs []*ExecSpec) error {
	if len(execSpecs) == 0 {
		return errors.New("exec_specs: cannot be empty")
	}

	seenIDs := make(map[string]struct{})
	for i, spec := range execSpecs {
		if err := spec.Validate(); err != nil {
			return errors.Wrapf(err, "exec_specs[%d]", i)
		}
		if pid := spec.GetPeerId(); pid != "" {
			if _, ok := seenIDs[pid]; ok {
				return errors.Errorf(
					"exec_specs[%d]: peer id %s appears multiple times",
					i,
					pid,
				)
			}
			seenIDs[pid] = struct{}{}
		}
	}

	return nil
}

// Validate validates the execution specification.
func (s *ExecSpec) Validate() error {
	if s.GetPeerId() == "" {
		return peer.ErrPeerIDEmpty
	}
	if _, err := s.ParsePeerID(); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the peer ID field.
func (s *ExecSpec) ParsePeerID() (peer.ID, error) {
	return confparse.ParsePeerID(s.GetPeerId())
}
