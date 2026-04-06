//go:build !js

package bldr_project_starlark

import (
	bldr_project "github.com/aperturerobotics/bldr/project"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// buildBuiltin implements the build() built-in function.
// build(id, manifests=[], targets=[], platformIds=[])
func (e *evaluator) buildBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id string
	var manifests []string
	var targets []string
	var platformIDs []string

	if len(args) > 1 {
		return nil, errors.New("build() accepts at most 1 positional argument (id)")
	}
	if len(args) == 1 {
		s, ok := args[0].(starlark.String)
		if !ok {
			return nil, errExpectedString("build", "id")
		}
		id = string(s)
	}

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		switch key {
		case "id":
			if id != "" {
				return nil, errors.New("build(): id specified both positionally and as keyword")
			}
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("build", "id")
			}
			id = string(s)
		case "manifests":
			list, err := toStringList(val)
			if err != nil {
				return nil, errors.Wrap(err, "build(): manifests")
			}
			manifests = list
		case "targets":
			list, err := toStringList(val)
			if err != nil {
				return nil, errors.Wrap(err, "build(): targets")
			}
			targets = list
		case "platformIds", "platform_ids":
			list, err := toStringList(val)
			if err != nil {
				return nil, errors.Wrap(err, "build(): platformIds")
			}
			platformIDs = list
		default:
			return nil, errUnknownKwarg("build", key)
		}
	}

	if id == "" {
		return nil, errors.New("build(): id is required")
	}

	bc := &bldr_project.BuildConfig{
		Manifests:   manifests,
		Targets:     targets,
		PlatformIds: platformIDs,
	}

	if e.config.Build == nil {
		e.config.Build = make(map[string]*bldr_project.BuildConfig)
	}
	e.config.Build[id] = bc

	return starlark.None, nil
}

// toStringList converts a Starlark list or tuple to a Go string slice.
func toStringList(val starlark.Value) ([]string, error) {
	switch v := val.(type) {
	case *starlark.List:
		result := make([]string, v.Len())
		for i := range v.Len() {
			s, ok := v.Index(i).(starlark.String)
			if !ok {
				return nil, errors.New("list must contain only strings")
			}
			result[i] = string(s)
		}
		return result, nil
	case starlark.Tuple:
		result := make([]string, len(v))
		for i, item := range v {
			s, ok := item.(starlark.String)
			if !ok {
				return nil, errors.New("tuple must contain only strings")
			}
			result[i] = string(s)
		}
		return result, nil
	default:
		return nil, errors.Errorf("expected list or tuple, got %s", val.Type())
	}
}
