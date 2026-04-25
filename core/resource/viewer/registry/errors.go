package resource_viewer_registry

import "errors"

// ErrRegistrationRequired is returned when no registration is provided.
var ErrRegistrationRequired = errors.New("registration is required")

// ErrTypeIdRequired is returned when the type_id field is empty.
var ErrTypeIdRequired = errors.New("type_id is required")

// ErrScriptPathRequired is returned when the script_path field is empty.
var ErrScriptPathRequired = errors.New("script_path is required")
