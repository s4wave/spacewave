package resource_worldop_registry

import (
	"context"
	"errors"
)

// ErrOperationTypeIdRequired is returned when the operation_type_id field is empty.
var ErrOperationTypeIdRequired = errors.New("operation_type_id is required")

// ErrPluginIdRequired is returned when the plugin_id field is empty.
var ErrPluginIdRequired = errors.New("plugin_id is required")

// ErrOpTypeIdMustHavePluginPrefix is returned when the operation_type_id has no namespace prefix before '/'.
var ErrOpTypeIdMustHavePluginPrefix = errors.New("operation_type_id must contain a namespace prefix before '/'")

// ErrOperationTypeAlreadyRegistered is returned when the operation_type_id already has an active registration.
var ErrOperationTypeAlreadyRegistered = errors.New("operation_type_id is already registered")

// PluginOpTimeoutError reports that a plugin did not answer a World operation
// within pluginOpDeadline. The transaction that held the operation is
// discarded and nothing it wrote is kept. It matches context.DeadlineExceeded,
// so a replay stops at the operation and tries it again rather than refusing
// it.
type PluginOpTimeoutError struct {
	// PluginID is the plugin that did not answer.
	PluginID string
	// OperationTypeID is the operation type it was applying.
	OperationTypeID string
}

// Error returns the timeout explanation.
func (e *PluginOpTimeoutError) Error() string {
	return "plugin " + e.PluginID + " did not apply operation " + e.OperationTypeID + " within " + pluginOpDeadline.String()
}

// Unwrap returns context.DeadlineExceeded.
func (e *PluginOpTimeoutError) Unwrap() error {
	return context.DeadlineExceeded
}
