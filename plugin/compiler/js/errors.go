package bldr_plugin_compiler_js

import (
	"github.com/pkg/errors"
)

// ErrUnexpectedVarType is returned when a variable has an unexpected type.
var ErrUnexpectedVarType = errors.New("unexpected variable type for build directive")
