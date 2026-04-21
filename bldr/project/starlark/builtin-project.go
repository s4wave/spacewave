//go:build !js

package bldr_project_starlark

import (
	bldr_project "github.com/s4wave/spacewave/bldr/project"
	"go.starlark.net/starlark"
)

// projectBuiltin implements the project() built-in function.
// Accepts kwargs: id, start, extends.
func (e *evaluator) projectBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, errNoPositionalArgs("project")
	}

	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		val := kv[1]
		switch key {
		case "id":
			s, ok := val.(starlark.String)
			if !ok {
				return nil, errExpectedString("project", "id")
			}
			e.config.Id = string(s)
		case "start":
			startConf, err := dictToStartConfig(val)
			if err != nil {
				return nil, err
			}
			e.config.Start = startConf
		case "extends":
			list, ok := val.(*starlark.List)
			if !ok {
				return nil, errExpectedList("project", "extends")
			}
			extends := make([]string, list.Len())
			for i := range list.Len() {
				s, ok := list.Index(i).(starlark.String)
				if !ok {
					return nil, errExpectedStringInList("project", "extends")
				}
				extends[i] = string(s)
			}
			e.config.Extends = extends
		default:
			return nil, errUnknownKwarg("project", key)
		}
	}

	return starlark.None, nil
}

// dictToStartConfig converts a Starlark dict or StartConfig-shaped value to a StartConfig.
func dictToStartConfig(val starlark.Value) (*bldr_project.StartConfig, error) {
	dict, ok := val.(*starlark.Dict)
	if !ok {
		return nil, errExpectedDict("project", "start")
	}

	conf := &bldr_project.StartConfig{}
	for _, item := range dict.Items() {
		key, ok := item[0].(starlark.String)
		if !ok {
			continue
		}
		switch string(key) {
		case "plugins":
			list, ok := item[1].(*starlark.List)
			if !ok {
				return nil, errExpectedList("start", "plugins")
			}
			for i := range list.Len() {
				s, ok := list.Index(i).(starlark.String)
				if !ok {
					return nil, errExpectedStringInList("start", "plugins")
				}
				conf.Plugins = append(conf.Plugins, string(s))
			}
		case "disableBuild", "disable_build":
			b, ok := item[1].(starlark.Bool)
			if !ok {
				return nil, errExpectedBool("start", "disableBuild")
			}
			conf.DisableBuild = bool(b)
		case "loadWebStartup", "load_web_startup":
			s, ok := item[1].(starlark.String)
			if !ok {
				return nil, errExpectedString("start", "loadWebStartup")
			}
			conf.LoadWebStartup = string(s)
		default:
			return nil, errUnknownField("start", string(key))
		}
	}

	return conf, nil
}
