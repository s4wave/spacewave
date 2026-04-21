package forge_target

import (
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
)

// IsNil checks if the object is nil.
func (e *Exec) IsNil() bool {
	return e == nil
}

// Validate performs cursory validation of the Exec.
func (e *Exec) Validate() error {
	if e.GetDisable() {
		return nil
	}
	if err := e.GetController().Validate(); err != nil {
		return errors.Wrap(err, "controller")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (e *Exec) MarshalBlock() ([]byte, error) {
	return e.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Exec) UnmarshalBlock(data []byte) error {
	return e.UnmarshalVT(data)
}

// _ is a type assertion
var (
	_ block.Block = ((*Exec)(nil))
)
