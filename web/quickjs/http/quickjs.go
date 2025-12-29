// Package quickjs_http provides QuickJS runtime files for HTTP serving.
package quickjs_http

import (
	plugin_host_wazero_quickjs "github.com/aperturerobotics/bldr/plugin/host/wazero-quickjs"
	quickjs_wasi "github.com/paralin/go-quickjs-wasi"
)

// QuickJSWASMBytes is the QuickJS WASI binary.
var QuickJSWASMBytes = quickjs_wasi.QuickJSWASM

// PluginQuickjsBootBytes is the boot harness for running plugins in QuickJS.
// This is the bundled TypeScript that sets up yamux, polyfills, etc.
var PluginQuickjsBootBytes []byte

func init() {
	// Read the boot harness from the embedded FS
	data, err := plugin_host_wazero_quickjs.PluginQuickjsBoot.ReadFile("plugin-quickjs.esm.js")
	if err != nil {
		panic("failed to read plugin-quickjs.esm.js: " + err.Error())
	}
	PluginQuickjsBootBytes = data
}
