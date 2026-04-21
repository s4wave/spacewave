package bldr_plugin_compiler_go

import (
	"github.com/pkg/errors"
)

// ErrUnexpectedVarType is returned when a variable has an unexpected type.
var ErrUnexpectedVarType = errors.New("unexpected variable type for build directive")
