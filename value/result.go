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

// NewResultSubBlockCtor returns the sub-block constructor.
func NewResultSubBlockCtor(r **Result) block.SubBlockCtor {
	if r == nil {
		return nil
	}
	return func(create bool) block.SubBlock {
		v := *r
		if create && v == nil {
			v = &Result{}
			*r = v
		}
		return v
	}
}

// Clone copies the result.
func (r *Result) Clone() *Result {
	if r == nil {
		return nil
	}

	return &Result{
		Success:   r.Success,
		FailError: r.FailError,
		Canceled:  r.Canceled,
	}
}

// IsEmpty checks if the result is empty.
func (r *Result) IsEmpty() bool {
	return !r.GetCanceled() &&
		r.GetFailError() == "" &&
		!r.GetSuccess()
}

// IsSuccessful checks if the result was successful.
func (r *Result) IsSuccessful() bool {
	return r.GetSuccess() &&
		len(r.GetFailError()) == 0 &&
		!r.GetCanceled()
}

// FillFailError fills the fail error with a default if it was unset.
func (r *Result) FillFailError() {
	if r != nil && len(r.GetFailError()) == 0 && !r.IsSuccessful() {
		if r.GetCanceled() {
			r.FailError = "canceled"
		} else {
			r.FailError = "failed without error details"
		}
	}
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
