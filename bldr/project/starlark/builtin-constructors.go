//go:build !js

package bldr_project_starlark

import (
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// configEntryBuiltin implements config_entry(id, rev, config=None).
// Returns a dict matching the ControllerConfig structure: {id, rev, config}.
func configEntryBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id string
	var rev int
	var config starlark.Value

	switch len(args) {
	case 3:
		config = args[2]
		fallthrough
	case 2:
		i, ok := args[1].(starlark.Int)
		if !ok {
			return nil, errExpectedInt("config_entry", "rev")
		}
		v, ok := i.Int64()
		if !ok {
			return nil, errors.New("config_entry(): rev value out of range")
		}
		rev = int(v)
		fallthrough
	case 1:
		s, ok := args[0].(starlark.String)
		if !ok {
			return nil, errExpectedString("config_entry", "id")
		}
		id = string(s)
	case 0:
		// handled by kwargs below
	default:
		return nil, errors.New("config_entry() accepts at most 3 positional arguments (id, rev, config)")
	}

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		switch key {
		case "id":
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("config_entry", "id")
			}
			id = string(s)
		case "rev":
			i, ok := val.(starlark.Int)
			if !ok {
				return nil, errExpectedInt("config_entry", "rev")
			}
			v, ok := i.Int64()
			if !ok {
				return nil, errors.New("config_entry(): rev value out of range")
			}
			rev = int(v)
		case "config":
			config = val
		default:
			return nil, errUnknownKwarg("config_entry", key)
		}
	}

	if id == "" {
		return nil, errors.New("config_entry(): id is required")
	}

	dict := starlark.NewDict(3)
	if err := dict.SetKey(starlark.String("id"), starlark.String(id)); err != nil {
		return nil, err
	}
	if err := dict.SetKey(starlark.String("rev"), starlark.MakeInt(rev)); err != nil {
		return nil, err
	}
	if config != nil && config != starlark.None {
		if err := dict.SetKey(starlark.String("config"), config); err != nil {
			return nil, err
		}
	}

	return dict, nil
}

// startConfigBuiltin implements start_config(plugins, loadWebStartup, disableBuild).
// Returns a dict matching StartConfig structure.
func startConfigBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, errNoPositionalArgs("start_config")
	}

	dict := starlark.NewDict(3)
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		switch key {
		case "plugins":
			if _, ok := val.(*starlark.List); !ok {
				return nil, errExpectedList("start_config", "plugins")
			}
			if err := dict.SetKey(starlark.String("plugins"), val); err != nil {
				return nil, err
			}
		case "loadWebStartup", "load_web_startup":
			if _, ok := val.(starlark.String); !ok {
				return nil, errExpectedString("start_config", "loadWebStartup")
			}
			if err := dict.SetKey(starlark.String("loadWebStartup"), val); err != nil {
				return nil, err
			}
		case "disableBuild", "disable_build":
			if _, ok := val.(starlark.Bool); !ok {
				return nil, errExpectedBool("start_config", "disableBuild")
			}
			if err := dict.SetKey(starlark.String("disableBuild"), val); err != nil {
				return nil, err
			}
		default:
			return nil, errUnknownKwarg("start_config", key)
		}
	}

	return dict, nil
}

// webPkgBuiltin implements web_pkg(id, exclude=False, entrypoints=None).
// Returns a dict matching WebPkgRefConfig structure.
// Entrypoints can be a list of strings (converted to [{path: s}, ...]).
func webPkgBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id string

	if len(args) > 1 {
		return nil, errors.New("web_pkg() accepts at most 1 positional argument (id)")
	}
	if len(args) == 1 {
		s, ok := args[0].(starlark.String)
		if !ok {
			return nil, errExpectedString("web_pkg", "id")
		}
		id = string(s)
	}

	var exclude starlark.Bool
	var entrypoints starlark.Value

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		switch key {
		case "id":
			if id != "" {
				return nil, errors.New("web_pkg(): id specified both positionally and as keyword")
			}
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("web_pkg", "id")
			}
			id = string(s)
		case "exclude":
			b, ok := val.(starlark.Bool)
			if !ok {
				return nil, errExpectedBool("web_pkg", "exclude")
			}
			exclude = b
		case "entrypoints":
			entrypoints = val
		default:
			return nil, errUnknownKwarg("web_pkg", key)
		}
	}

	if id == "" {
		return nil, errors.New("web_pkg(): id is required")
	}

	dict := starlark.NewDict(3)
	if err := dict.SetKey(starlark.String("id"), starlark.String(id)); err != nil {
		return nil, err
	}
	if exclude {
		if err := dict.SetKey(starlark.String("exclude"), starlark.True); err != nil {
			return nil, err
		}
	}
	if entrypoints != nil && entrypoints != starlark.None {
		// Convert list of strings to list of {path: s} dicts for WebPkgEntrypoint.
		if list, ok := entrypoints.(*starlark.List); ok {
			epList := starlark.NewList(nil)
			for i := range list.Len() {
				item := list.Index(i)
				if s, ok := item.(starlark.String); ok {
					d := starlark.NewDict(1)
					_ = d.SetKey(starlark.String("path"), s)
					if err := epList.Append(d); err != nil {
						return nil, err
					}
				} else {
					// Already a dict or other structure, pass through.
					if err := epList.Append(item); err != nil {
						return nil, err
					}
				}
			}
			if err := dict.SetKey(starlark.String("entrypoints"), epList); err != nil {
				return nil, err
			}
		} else {
			if err := dict.SetKey(starlark.String("entrypoints"), entrypoints); err != nil {
				return nil, err
			}
		}
	}

	return dict, nil
}

// jsModuleBuiltin implements js_module(kind, path, **kwargs).
// Returns a dict matching JsModule structure.
func jsModuleBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var kind string
	var modulePath string

	switch len(args) {
	case 2:
		s, ok := args[1].(starlark.String)
		if !ok {
			return nil, errExpectedString("js_module", "path")
		}
		modulePath = string(s)
		fallthrough
	case 1:
		s, ok := args[0].(starlark.String)
		if !ok {
			return nil, errExpectedString("js_module", "kind")
		}
		kind = string(s)
	case 0:
		// handled by kwargs
	default:
		return nil, errors.New("js_module() accepts at most 2 positional arguments (kind, path)")
	}

	dict := starlark.NewDict(4)
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		switch key {
		case "kind":
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("js_module", "kind")
			}
			kind = string(s)
		case "path":
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("js_module", "path")
			}
			modulePath = string(s)
		default:
			// pass through additional kwargs to the dict
			if err := dict.SetKey(starlark.String(key), val); err != nil {
				return nil, err
			}
		}
	}

	if kind == "" {
		return nil, errors.New("js_module(): kind is required")
	}
	if modulePath == "" {
		return nil, errors.New("js_module(): path is required")
	}

	if err := dict.SetKey(starlark.String("kind"), starlark.String(kind)); err != nil {
		return nil, err
	}
	if err := dict.SetKey(starlark.String("path"), starlark.String(modulePath)); err != nil {
		return nil, err
	}

	return dict, nil
}

// remoteBuiltin implements the remote() registration built-in.
// remote(id, engineId, peerId, objectKey, hostConfigSet={}, linkObjectKeys=[])
func (e *evaluator) remoteBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id string

	if len(args) > 1 {
		return nil, errors.New("remote() accepts at most 1 positional argument (id)")
	}
	if len(args) == 1 {
		s, ok := args[0].(starlark.String)
		if !ok {
			return nil, errExpectedString("remote", "id")
		}
		id = string(s)
	}

	// Collect all kwargs into a dict for JSON conversion.
	fields := starlark.NewDict(len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		if key == "id" {
			if id != "" {
				return nil, errors.New("remote(): id specified both positionally and as keyword")
			}
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("remote", "id")
			}
			id = string(s)
			continue
		}
		if err := fields.SetKey(kv[0], val); err != nil {
			return nil, err
		}
	}

	if id == "" {
		return nil, errors.New("remote(): id is required")
	}

	jsonData, err := valueToJSON(fields)
	if err != nil {
		return nil, errors.Wrap(err, "remote()")
	}

	conf := &bldr_project.RemoteConfig{}
	if err := conf.UnmarshalJSON(jsonData); err != nil {
		return nil, errors.Wrap(err, "remote(): invalid config")
	}

	if e.config.Remotes == nil {
		e.config.Remotes = make(map[string]*bldr_project.RemoteConfig)
	}
	e.config.Remotes[id] = conf

	return starlark.None, nil
}

// publishBuiltin implements the publish() registration built-in.
// publish(id, **kwargs)
func (e *evaluator) publishBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var id string

	if len(args) > 1 {
		return nil, errors.New("publish() accepts at most 1 positional argument (id)")
	}
	if len(args) == 1 {
		s, ok := args[0].(starlark.String)
		if !ok {
			return nil, errExpectedString("publish", "id")
		}
		id = string(s)
	}

	fields := starlark.NewDict(len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		if key == "id" {
			if id != "" {
				return nil, errors.New("publish(): id specified both positionally and as keyword")
			}
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("publish", "id")
			}
			id = string(s)
			continue
		}
		if err := fields.SetKey(kv[0], val); err != nil {
			return nil, err
		}
	}

	if id == "" {
		return nil, errors.New("publish(): id is required")
	}

	jsonData, err := valueToJSON(fields)
	if err != nil {
		return nil, errors.Wrap(err, "publish()")
	}

	conf := &bldr_project.PublishConfig{}
	if err := conf.UnmarshalJSON(jsonData); err != nil {
		return nil, errors.Wrap(err, "publish(): invalid config")
	}

	if e.config.Publish == nil {
		e.config.Publish = make(map[string]*bldr_project.PublishConfig)
	}
	e.config.Publish[id] = conf

	return starlark.None, nil
}
