package world

import (
	"context"
	"time"
)

// PluginOpTimeoutError reports that a plugin did not answer a World operation
// within Deadline: the WriteBudget of the write holding the engine's writer,
// shared by the write's attempts and nested operations, or the per-operation
// deadline of an operation applied without a write budget. The apply returns
// it. When the World state the operation was applied to discards a failed
// operation's transaction, as AppliesAtomically reports, nothing the
// operation wrote was kept. It matches context.DeadlineExceeded, so a replay
// stops at the operation and tries it again rather than refusing it.
type PluginOpTimeoutError struct {
	// PluginID is the plugin that did not answer.
	PluginID string
	// OperationTypeID is the operation type it was applying.
	OperationTypeID string
	// Deadline is the length of the budget or deadline that ended the call.
	Deadline time.Duration
}

// Error returns the timeout explanation.
func (e *PluginOpTimeoutError) Error() string {
	return "plugin " + e.PluginID + " did not apply operation " + e.OperationTypeID + " within " + e.Deadline.String()
}

// Unwrap returns context.DeadlineExceeded.
func (e *PluginOpTimeoutError) Unwrap() error {
	return context.DeadlineExceeded
}
