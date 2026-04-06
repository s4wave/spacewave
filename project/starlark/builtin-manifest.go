//go:build !js

package bldr_project_starlark

import (
	bldr_project "github.com/aperturerobotics/bldr/project"
	manifest "github.com/aperturerobotics/bldr/manifest"
	configset_proto "github.com/aperturerobotics/controllerbus/controller/configset/proto"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// manifestBuiltin implements the manifest() built-in function.
// manifest(id, builder, rev=0, config=None, description="")
func (e *evaluator) manifestBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id string
	var builder string
	var rev int
	var config starlark.Value
	var description string

	// Parse first positional arg as id if present.
	if len(args) > 1 {
		return nil, errors.New("manifest() accepts at most 1 positional argument (id)")
	}
	if len(args) == 1 {
		s, ok := args[0].(starlark.String)
		if !ok {
			return nil, errExpectedString("manifest", "id")
		}
		id = string(s)
	}

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		switch key {
		case "id":
			if id != "" {
				return nil, errors.New("manifest(): id specified both positionally and as keyword")
			}
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("manifest", "id")
			}
			id = string(s)
		case "builder":
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("manifest", "builder")
			}
			builder = string(s)
		case "rev":
			i, ok := val.(starlark.Int)
			if !ok {
				return nil, errExpectedInt("manifest", "rev")
			}
			v, ok := i.Int64()
			if !ok {
				return nil, errors.New("manifest(): rev value out of range")
			}
			rev = int(v)
		case "config":
			config = val
		case "description":
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("manifest", "description")
			}
			description = string(s)
		default:
			return nil, errUnknownKwarg("manifest", key)
		}
	}

	if id == "" {
		return nil, errors.New("manifest(): id is required")
	}
	if err := manifest.ValidateManifestID(id, false); err != nil {
		return nil, errors.Wrap(err, "manifest()")
	}
	if builder == "" {
		return nil, errors.New("manifest(): builder is required")
	}

	// Build the ControllerConfig for the builder.
	// The rev goes on the ControllerConfig (builder cache buster),
	// not on ManifestConfig.Rev (minimum manifest revision).
	ctrlConf := &configset_proto.ControllerConfig{
		Id:  builder,
		Rev: uint64(rev),
	}
	if config != nil && config != starlark.None {
		configJSON, err := valueToJSON(config)
		if err != nil {
			return nil, errors.Wrap(err, "manifest(): config")
		}
		ctrlConf.Config = configJSON
	}

	mc := &bldr_project.ManifestConfig{
		Builder:     ctrlConf,
		Description: description,
	}

	if e.config.Manifests == nil {
		e.config.Manifests = make(map[string]*bldr_project.ManifestConfig)
	}
	e.config.Manifests[id] = mc

	return starlark.None, nil
}
