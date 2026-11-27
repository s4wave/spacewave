//go:build !js

package bldr_project_starlark

import (
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// varBuiltin implements the var() built-in function: var(name, default)
// returns the input the caller passed under name, or default when it passed
// none. Without a default, a missing input is an error.
func (e *evaluator) varBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	// Read the name and the optional default value.
	var name string
	var def starlark.Value
	if err := starlark.UnpackArgs("var", args, kwargs, "name", &name, "default?", &def); err != nil {
		return nil, err
	}
	if name == "" {
		return nil, errors.New("var(): name must not be empty")
	}

	// Return the caller's value, else the default value.
	value, ok, err := e.lookupVar(name)
	if err != nil {
		return nil, errors.Wrapf(err, "var(%q)", name)
	}
	if ok {
		return starlark.String(value), nil
	}
	if def == nil {
		return nil, errors.Errorf("var(): input %q was not passed and has no default", name)
	}
	return def, nil
}
