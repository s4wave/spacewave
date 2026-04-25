package resource_worldop_registry

import "errors"

// ErrOperationTypeIdRequired is returned when the operation_type_id field is empty.
var ErrOperationTypeIdRequired = errors.New("operation_type_id is required")

// ErrPluginIdRequired is returned when the plugin_id field is empty.
var ErrPluginIdRequired = errors.New("plugin_id is required")

// ErrOpTypeIdMustHavePluginPrefix is returned when the operation_type_id has no namespace prefix before '/'.
var ErrOpTypeIdMustHavePluginPrefix = errors.New("operation_type_id must contain a namespace prefix before '/'")
