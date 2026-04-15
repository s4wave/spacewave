//go:build !js

package bldr_project_starlark

import (
	"github.com/pkg/errors"
	"go.starlark.net/starlark"
)

// validGoPluginFields are the valid field names for go_plugin_config().
var validGoPluginFields = map[string]bool{
	"goPkgs":                      true,
	"go_pkgs":                     true,
	"configSet":                   true,
	"config_set":                  true,
	"hostConfigSet":               true,
	"host_config_set":             true,
	"webPkgs":                     true,
	"web_pkgs":                    true,
	"buildTypes":                  true,
	"build_types":                 true,
	"platformTypes":               true,
	"platform_types":              true,
	"webPluginId":                 true,
	"web_plugin_id":               true,
	"projectId":                   true,
	"project_id":                  true,
	"viteConfigPaths":             true,
	"vite_config_paths":           true,
	"viteDisableProjectConfig":    true,
	"vite_disable_project_config": true,
	"disableRpcFetch":             true,
	"disable_rpc_fetch":           true,
	"delveAddr":                   true,
	"delve_addr":                  true,
	"enableCgo":                   true,
	"enable_cgo":                  true,
	"enableTinygo":                true,
	"enable_tinygo":               true,
	"enableCompression":           true,
	"enable_compression":          true,
	"esbuildFlags":                true,
	"esbuild_flags":               true,
}

// goPluginConfigBuiltin implements go_plugin_config(**kwargs).
// Returns a dict with validated fields for the Go plugin compiler config.
func goPluginConfigBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return buildTypedConfig("go_plugin_config", validGoPluginFields, args, kwargs)
}

// validJsPluginFields are the valid field names for js_plugin_config().
var validJsPluginFields = map[string]bool{
	"modules":                     true,
	"esbuildBundles":              true,
	"esbuild_bundles":             true,
	"esbuildFlags":                true,
	"esbuild_flags":               true,
	"viteBundles":                 true,
	"vite_bundles":                true,
	"viteConfigPaths":             true,
	"vite_config_paths":           true,
	"viteDisableProjectConfig":    true,
	"vite_disable_project_config": true,
	"backendEntrypoints":          true,
	"backend_entrypoints":         true,
	"frontendEntrypoints":         true,
	"frontend_entrypoints":        true,
	"webPkgs":                     true,
	"web_pkgs":                    true,
	"hostConfigSet":               true,
	"host_config_set":             true,
	"disableRpcFetch":             true,
	"disable_rpc_fetch":           true,
	"webPluginId":                 true,
	"web_plugin_id":               true,
	"buildTypes":                  true,
	"build_types":                 true,
	"platformTypes":               true,
	"platform_types":              true,
}

// jsPluginConfigBuiltin implements js_plugin_config(**kwargs).
func jsPluginConfigBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return buildTypedConfig("js_plugin_config", validJsPluginFields, args, kwargs)
}

// validCliCompilerFields are the valid field names for cli_compiler_config().
var validCliCompilerFields = map[string]bool{
	"goPkgs":     true,
	"go_pkgs":    true,
	"cliPkgs":    true,
	"cli_pkgs":   true,
	"configSet":  true,
	"config_set": true,
	"projectId":  true,
	"project_id": true,
}

// cliCompilerConfigBuiltin implements cli_compiler_config(**kwargs).
func cliCompilerConfigBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return buildTypedConfig("cli_compiler_config", validCliCompilerFields, args, kwargs)
}

// validDistCompilerFields are the valid field names for dist_compiler_config().
var validDistCompilerFields = map[string]bool{
	"embedManifests":     true,
	"embed_manifests":    true,
	"loadPlugins":        true,
	"load_plugins":       true,
	"loadWebStartup":     true,
	"load_web_startup":   true,
	"hostConfigSet":      true,
	"host_config_set":    true,
	"projectId":          true,
	"project_id":         true,
	"enableCgo":          true,
	"enable_cgo":         true,
	"enableTinygo":       true,
	"enable_tinygo":      true,
	"enableCompression":  true,
	"enable_compression": true,
}

// distCompilerConfigBuiltin implements dist_compiler_config(**kwargs).
func distCompilerConfigBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return buildTypedConfig("dist_compiler_config", validDistCompilerFields, args, kwargs)
}

// validWebPluginCompilerFields are the valid field names for web_plugin_compiler_config().
var validWebPluginCompilerFields = map[string]bool{
	"nativeApp":    true,
	"native_app":   true,
	"projectId":    true,
	"project_id":   true,
	"delveAddr":    true,
	"delve_addr":   true,
	"electronPkg":  true,
	"electron_pkg": true,
}

// webPluginCompilerConfigBuiltin implements web_plugin_compiler_config(**kwargs).
func webPluginCompilerConfigBuiltin(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	return buildTypedConfig("web_plugin_compiler_config", validWebPluginCompilerFields, args, kwargs)
}

// buildTypedConfig validates kwargs against allowed fields and returns a dict.
func buildTypedConfig(fnName string, validFields map[string]bool, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 0 {
		return nil, errNoPositionalArgs(fnName)
	}

	dict := starlark.NewDict(len(kwargs))
	for _, kv := range kwargs {
		key := string(kv[0].(starlark.String))
		if !validFields[key] {
			return nil, errors.Errorf("%s(): unknown field %q", fnName, key)
		}
		if err := dict.SetKey(kv[0], kv[1]); err != nil {
			return nil, err
		}
	}

	return dict, nil
}
