package forge_execution

import "errors"

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
