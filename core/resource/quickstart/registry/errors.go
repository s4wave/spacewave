package resource_quickstart_registry

import "errors"

// ErrRegistrationRequired is returned when no registration is provided.
var ErrRegistrationRequired = errors.New("registration is required")

// ErrQuickstartIdRequired is returned when the quickstart_id field is empty.
var ErrQuickstartIdRequired = errors.New("quickstart_id is required")

// ErrQuickstartIdAlreadyRegistered is returned when quickstart_id is already registered.
var ErrQuickstartIdAlreadyRegistered = errors.New("quickstart_id is already registered")

// ErrQuickstartNotRegistered is returned when quickstart_id is not registered.
var ErrQuickstartNotRegistered = errors.New("quickstart_id is not registered")

// ErrPluginIdRequired is returned when the plugin_id field is empty.
var ErrPluginIdRequired = errors.New("plugin_id is required")

// ErrNameRequired is returned when the name field is empty.
var ErrNameRequired = errors.New("name is required")

// ErrDescriptionRequired is returned when the description field is empty.
var ErrDescriptionRequired = errors.New("description is required")

// ErrCategoryRequired is returned when the category field is empty.
var ErrCategoryRequired = errors.New("category is required")

// ErrSpaceResourceIdRequired is returned when space_resource_id is empty.
var ErrSpaceResourceIdRequired = errors.New("space_resource_id is required")

// ErrSpaceResourceRequired is returned when the resource is not a Space resource.
var ErrSpaceResourceRequired = errors.New("space resource is required")

// ErrQuickstartExecutionUnavailable is returned when no plugin bus is configured.
var ErrQuickstartExecutionUnavailable = errors.New("quickstart execution is unavailable")
