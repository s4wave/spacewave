package forge_value

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
	"github.com/golang/protobuf/proto"
)

// NewResultWithSuccess constructs a new result.
func NewResultWithSuccess() *Result {
	return &Result{Success: true}
}

// NewResultWithError constructs a new result with an error.
// If err == nil, returns NewResultWithSuccess.
func NewResultWithError(err error) *Result {
	if err == nil {
		return NewResultWithSuccess()
	}
	return &Result{FailError: err.Error()}
}

// Validate performs cursory checks of the Result.
func (r *Result) Validate() error {
	if len(r.GetFailError()) != 0 {
		if r.GetSuccess() {
			return errors.New("expected empty fail_error for successful result")
		}
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (r *Result) MarshalBlock() ([]byte, error) {
	return proto.Marshal(r)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (r *Result) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, r)
}

// _ is a type assertion
var _ block.Block = ((*Result)(nil))
