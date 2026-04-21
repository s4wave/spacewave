//go:build !js

package bldr_project_starlark

import (
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"go.starlark.net/starlark"
)

// buildBuiltin implements the build() built-in function.
// build(id, manifests=[], targets=[], platformIds=[], manifestOverrides={})
func (e *evaluator) buildBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id string
	var manifests []string
	var targets []string
	var platformIDs []string
	var manifestOverridesRaw starlark.IterableMapping

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
		case "manifestOverrides", "manifest_overrides":
			m, ok := val.(starlark.IterableMapping)
			if !ok {
				return nil, errors.Errorf("build(): manifestOverrides: expected dict, got %s", val.Type())
			}
			manifestOverridesRaw = m
		default:
			return nil, errUnknownKwarg("build", key)
		}
	}

	if id == "" {
		return nil, errors.New("build(): id is required")
	}

	manifestOverrides, err := parseManifestOverrides(manifestOverridesRaw)
	if err != nil {
		return nil, err
	}

	bc := &bldr_project.BuildConfig{
		Manifests:         manifests,
		Targets:           targets,
		PlatformIds:       platformIDs,
		ManifestOverrides: manifestOverrides,
	}

	if e.config.Build == nil {
		e.config.Build = make(map[string]*bldr_project.BuildConfig)
	}
	e.config.Build[id] = bc

	return starlark.None, nil
}

// parseManifestOverrides parses a starlark.IterableMapping of manifest ids to
// inner-config dicts into a map of ControllerConfig protos. Each value is the
// builder config payload (e.g. =dist_compiler_config(embedManifests=[...])=);
// valueToJSON encodes it and the bytes become =ControllerConfig.Config=. The
// override's id is deliberately left empty because the manifest's declared
// builder id wins at apply time (see project/controller/manifest-builder.go).
func parseManifestOverrides(val starlark.IterableMapping) (map[string]*configset_proto.ControllerConfig, error) {
	if val == nil {
		return nil, nil
	}
	items := val.Items()
	if len(items) == 0 {
		return nil, nil
	}
	out := make(map[string]*configset_proto.ControllerConfig, len(items))
	for _, item := range items {
		keyStr, ok := item[0].(starlark.String)
		if !ok {
			return nil, errors.Errorf("build(): manifestOverrides key must be a string, got %s", item[0].Type())
		}
		manifestID := string(keyStr)
		if manifestID == "" {
			return nil, errors.New("build(): manifestOverrides key must be non-empty")
		}
		dict, ok := item[1].(*starlark.Dict)
		if !ok {
			return nil, errors.Errorf("build(): manifestOverrides[%q]: expected dict, got %s", manifestID, item[1].Type())
		}
		jsonData, err := valueToJSON(dict)
		if err != nil {
			return nil, errors.Wrapf(err, "build(): manifestOverrides[%q]", manifestID)
		}
		out[manifestID] = &configset_proto.ControllerConfig{Config: jsonData}
	}
	return out, nil
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
