package s4wave_wizard

import (
	"context"

	"github.com/s4wave/spacewave/db/block"
)

// NewWizardStateBlock constructs a new WizardState block.
func NewWizardStateBlock() block.Block {
	return &WizardState{}
}

// UnmarshalWizardState unmarshals a wizard state from a cursor.
func UnmarshalWizardState(ctx context.Context, bcs *block.Cursor) (*WizardState, error) {
	return block.UnmarshalBlock[*WizardState](ctx, bcs, NewWizardStateBlock)
}

// Validate performs cursory checks on the WizardState block.
func (s *WizardState) Validate() error {
	return nil
}

// MarshalBlock marshals the block to binary.
func (s *WizardState) MarshalBlock() ([]byte, error) {
	return s.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
func (s *WizardState) UnmarshalBlock(data []byte) error {
	return s.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*WizardState)(nil))
