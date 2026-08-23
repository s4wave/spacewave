package bldr_project_starlark

import (
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// popPositionalID consumes at most one positional argument as an id and
// removes any "id" kwarg from kwargs, erroring when the id is specified
// both ways.
func popPositionalID(
	fnName string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, []starlark.Tuple, error) {
	var id string
	if len(args) > 1 {
		return "", nil, errors.Errorf("%s() accepts at most 1 positional argument (id)", fnName)
	}
	if len(args) == 1 {
		s, ok := args[0].(starlark.String)
		if !ok {
			return "", nil, errExpectedString(fnName, "id")
		}
		id = string(s)
	}

	remaining := make([]starlark.Tuple, 0, len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if key != "id" {
			remaining = append(remaining, kv)
			continue
		}
		if id != "" {
			return "", nil, errors.New(fnName + "(): id specified both positionally and as keyword")
		}
		s, ok := kv[1].(starlark.String)
		if !ok {
			return "", nil, errExpectedString(fnName, "id")
		}
		id = string(s)
	}
	return id, remaining, nil
}
