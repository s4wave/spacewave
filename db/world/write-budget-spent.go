package world

import (
	"context"
	"time"
)

// WriteBudgetSpentError reports that a World write stopped replaying because
// a plugin call spent its WriteBudget. Nothing the write did was kept. It
// matches context.DeadlineExceeded.
type WriteBudgetSpentError struct {
	// Length is the budget's length.
	Length time.Duration
	// Err is the error of the last attempt.
	Err error
}

// Error returns the explanation.
func (e *WriteBudgetSpentError) Error() string {
	return "write budget of " + e.Length.String() + " spent: " + e.Err.Error()
}

// Unwrap returns context.DeadlineExceeded.
func (e *WriteBudgetSpentError) Unwrap() error {
	return context.DeadlineExceeded
}

// _ is a type assertion
var _ error = (*WriteBudgetSpentError)(nil)
