// Package plugin_host defines the PluginHost, an environment that runs plugin
// entrypoints for one platform ID.
//
// A PluginHost reports its platform ID, lists its loaded plugins, and executes
// a plugin from its entrypoint, its distribution and asset filesystems, and an
// RPC mux that connects the plugin to the host. The scheduler calls
// ExecutePlugin at most once at a time per plugin ID, with an execution key
// that tells apart a replacement starting before its predecessor stops. The
// subpackages are the hosts themselves: process runs native executables,
// wazero-quickjs runs JavaScript plugins natively, and web runs WebAssembly and
// JavaScript plugins in browser workers through the WebRuntime; default
// assembles the set for each runtime. The scheduler subpackage decides what
// runs where.
package plugin_host
