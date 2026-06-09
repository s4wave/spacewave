package sobject_world_engine

import (
	"errors"

	"github.com/s4wave/spacewave/db/block"
)

// NewSOWorldOpBlock constructs a new SOWorldOp block.
func NewSOWorldOpBlock() block.Block {
	return &SOWorldOp{}
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (s *SOWorldOp) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (s *SOWorldOp) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// Validate validates the InitWorldOp configuration.
func (i *InitWorldOp) Validate() error {
	if err := i.GetTransformConf().Validate(); err != nil {
		return err
	}
	return nil
}

// Validate checks the follower finalization packet has the fields required for
// authority-side stale-base and candidate-availability decisions.
func (p *SpaceWorldFinalizationPacket) Validate() error {
	if p == nil {
		return errors.New("finalization packet is nil")
	}
	if p.GetBaseSharedObjectRoot() == nil {
		return errors.New("base SharedObject root is required")
	}
	if p.GetBaseWorldRoot().GetEmpty() {
		return errors.New("base World root is required")
	}
	if p.GetCandidateWorldRoot().GetEmpty() {
		return errors.New("candidate World root is required")
	}
	if len(p.GetCandidateContentId()) == 0 {
		return errors.New("candidate content id is required")
	}
	if p.GetOp() == nil {
		return errors.New("candidate SharedObject World op is required")
	}
	return nil
}

// Validate checks accepted decisions carry the roots followers need to refresh.
func (d *SpaceWorldFinalizationDecision) Validate() error {
	if d == nil {
		return errors.New("finalization decision is nil")
	}
	switch d.GetStatus() {
	case SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_ACCEPTED:
		if d.GetAcceptedSharedObjectRoot() == nil {
			return errors.New("accepted SharedObject root is required")
		}
		if d.GetAcceptedWorldRoot().GetEmpty() {
			return errors.New("accepted World root is required")
		}
	case SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_REJECTED,
		SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_STALE_BASE,
		SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_MISSING_BLOCK,
		SpaceWorldFinalizationStatus_SPACE_WORLD_FINALIZATION_STATUS_LOST_AUTHORITY:
		return nil
	default:
		return errors.New("finalization decision status is required")
	}
	return nil
}

// _ is a type assertion
var _ block.Block = ((*SOWorldOp)(nil))
