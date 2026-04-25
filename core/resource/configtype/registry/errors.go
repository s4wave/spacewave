package resource_configtype_registry

import "errors"

// ErrConfigIdRequired is returned when the config_id field is empty.
var ErrConfigIdRequired = errors.New("config_id is required")

// ErrPluginIdRequired is returned when the plugin_id field is empty.
var ErrPluginIdRequired = errors.New("plugin_id is required")

// ErrScriptPathRequired is returned when the script_path field is empty.
var ErrScriptPathRequired = errors.New("script_path is required for plugin-registered config types")
