package plugin

import "errors"

var (
	// ErrEmptyPluginID is returned if the plugin ID was empty.
	ErrEmptyPluginID = errors.New("plugin id cannot be empty")
	// ErrEmptyPlatformID is returned if the platform ID was empty.
	ErrEmptyPlatformID = errors.New("platform id cannot be empty")
	// ErrEmptyEntrypoint is returned if the entrypoint was empty.
	ErrEmptyEntrypoint = errors.New("entrypoint cannot be empty")
	// ErrEmptyPath is returned if the path was empty.
	ErrEmptyPath = errors.New("path cannot be empty")
)
