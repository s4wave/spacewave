package bldr_plugin

import "github.com/pkg/errors"

// ErrEmptyPluginID is returned if the plugin ID was empty.
var ErrEmptyPluginID = errors.New("plugin id cannot be empty")
