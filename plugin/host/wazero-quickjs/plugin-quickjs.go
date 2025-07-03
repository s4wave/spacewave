package plugin_host_wazero_quickjs

import "embed"

// PluginQuickjsBoot contains the js script for the entrypoint for quickjs plugins.
// Mounted to /boot in the vm.
//
//go:generate go run -v ./gen/main.go
//go:embed plugin-quickjs.esm.js
var PluginQuickjsBoot embed.FS
