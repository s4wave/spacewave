package world

import "context"

// ProvisionalBaseError is an ErrSuperseded for a write that changed nothing on
// a base the Authority has not decided. The base may still be refused, so the
// write's finding that nothing needed to change is unproven. Wait returns once
// the base is decided, and a retry before then meets the same base.
type ProvisionalBaseError struct {
	// Wait waits for the base's decision, returning only ctx's error.
	Wait func(ctx context.Context) error
}

// Error describes the refused write.
func (e *ProvisionalBaseError) Error() string {
	return "unchanged write on provisional World head: " + ErrSuperseded.Error()
}

// Is reports that the error is an ErrSuperseded.
func (e *ProvisionalBaseError) Is(target error) bool {
	return target == ErrSuperseded
}
