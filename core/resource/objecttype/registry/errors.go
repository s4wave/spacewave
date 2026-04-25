package resource_objecttype_registry

import "errors"

// ErrTypeIdRequired is returned when the type_id field is empty.
var ErrTypeIdRequired = errors.New("type_id is required")

// ErrPluginIdRequired is returned when the plugin_id field is empty.
var ErrPluginIdRequired = errors.New("plugin_id is required")

// ErrTypeIdMustHavePluginPrefix is returned when the type_id has no namespace prefix before '/'.
var ErrTypeIdMustHavePluginPrefix = errors.New("type_id must contain a namespace prefix before '/'")
