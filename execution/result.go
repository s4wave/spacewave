package forge_execution

import "errors"

// NewResultWithSuccess constructs a new result.
func NewResultWithSuccess() *Result {
	return &Result{Success: true}
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
