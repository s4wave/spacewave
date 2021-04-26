package forge_target

import (
	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
	"github.com/pkg/errors"
)

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
	return proto.Marshal(e)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (e *Exec) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, e)
}

// _ is a type assertion
var (
	_ block.Block = ((*Exec)(nil))
)
